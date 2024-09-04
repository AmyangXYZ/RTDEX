
from packet_pb2 import (Acknowledgement, DataContent, DataInterest,
                        DataRegister, ErrorCode, JoinRequest,
                        RTDEXPacket, PacketHeader, PacketType, Priority)
import random
import socket
import time
import zlib
import select
from typing import Optional, Tuple


class RTDEX_API:
    def __init__(self, id, namespace, server_addr: Tuple[str, int] = ('localhost', 9999), verbose=False):
        self.id = id
        self.namespace = namespace
        self.sequence_number = -1
        self.session_token: Optional[str] = None
        self.conn = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        self.server_addr = server_addr
        self.connected = False
        self.verbose = verbose

        self.pkt_buf_size = 36*1024
        self.chunk_size = 32*1024
        self.max_retries = 3
        self.retry_delay = 1.0
        self.ack_timeout = 1
        self.response_timeout = 3.0

    def connect(self, authentication_token: int):
        """Connect to the RTDEX server."""
        try:
            join_response = self._join(authentication_token)
            self.session_token = join_response.session_token
            self.connected = True
            if self.verbose:
                print(f"[Client-{self.id}] Connected to RTDEX server")
            return join_response
        except Exception as e:
            print(f"[Client-{self.id}] Error connecting to RTDEX server: {e}")
            return False

    def disconnect(self):
        """Disconnect from the RTDEX server."""
        self.connected = False
        self.conn.close()
        if self.verbose:
            print(f"[Client-{self.id}] Disconnected from RTDEX server")

    def put(self, name: str, data: bytes, freshness: int = 300):
        """Upload data to the RTDEX server."""
        if not self.connected:
            raise RuntimeError("Not connected to RTDEX server")
        self._register_and_upload_data(name, data, freshness)

    def get(self, name: str, timeout: float = 30.0) -> Optional[bytes]:
        """Request data from the RTDEX server."""
        if not self.connected:
            raise RuntimeError("Not connected to RTDEX server")

        interest_packet = RTDEXPacket(
            header=self._create_header(PacketType.DATA_INTEREST, Priority.HIGH),
            data_interest=DataInterest(name=name)
        )

        if not self._send_packet(interest_packet):
            if self.verbose:
                print(f"[Client-{self.id}] Failed to send DATA_INTEREST after maximum retries")
            return None

        # Wait for DATA_CONTENT or ERROR_MESSAGE
        response = self._wait_for_packet([PacketType.DATA_CONTENT, PacketType.ERROR_MESSAGE], timeout)
        if not response:
            if self.verbose:
                print(f"[Client-{self.id}] No response received for DATA_INTEREST")
            return None

        if response.header.packet_type == PacketType.ERROR_MESSAGE:
            error_code = ErrorCode.Name(response.error_message.error_code)
            if self.verbose:
                print(f"[Client-{self.id}] Error retrieving data: {error_code}")
            return None

        if zlib.crc32(response.data_content.data) != response.data_content.checksum:
            if self.verbose:
                print(f"[Client-{self.id}] Data integrity check failed")
            return None

        return response.data_content.data

    def _create_header(self, packet_type, priority, uid=None):
        self.sequence_number += 1
        if uid is None:
            uid = random.randint(0, 2**32 - 1)

        return PacketHeader(
            protocol_version=1,
            packet_uid=uid,
            packet_type=packet_type,
            sequence_number=self.sequence_number,
            source_id=self.id,
            destination_id=1,
            priority=priority,
            timestamp=int(time.time() * 1e9),
            payload_length=0
        )

    def _send_packet(self, packet) -> bool:
        packet.header.payload_length = packet.ByteSize() - packet.header.ByteSize()
        serialized_packet = packet.SerializeToString()

        needs_ack = (packet.header.priority > Priority.LOW and
                     packet.header.packet_type not in [PacketType.ACKNOWLEDGEMENT, PacketType.ERROR_MESSAGE])

        for attempt in range(self.max_retries):
            self.conn.sendto(serialized_packet, self.server_addr)
            if self.verbose:
                print(f'[Client-{self.id}] Send {PacketType.Name(packet.header.packet_type)}-0x{packet.header.packet_uid:X}')

            if not needs_ack:
                return True

            # Wait for ACK
            ack = self._wait_for_packet(PacketType.ACKNOWLEDGEMENT, self.ack_timeout)
            if ack:
                return True

            print(f"[Client-{self.id}] No ACK received for {PacketType.Name(packet.header.packet_type)}, retrying...")

        return False

    def _receive_packet(self, timeout=1.0):
        ready = select.select([self.conn], [], [], timeout)
        if ready[0]:
            try:
                data, _ = self.conn.recvfrom(self.pkt_buf_size)
                packet = RTDEXPacket()
                packet.ParseFromString(data)
                if self.verbose:
                    print(
                        f'[Client-{self.id}] Received {PacketType.Name(packet.header.packet_type)}-0x{packet.header.packet_uid:X}')

                if packet.header.priority > Priority.LOW and packet.header.packet_type != PacketType.ACKNOWLEDGEMENT and packet.header.packet_type != PacketType.ERROR_MESSAGE:
                    self._send_ack(packet)

                return packet
            except Exception as e:
                if self.verbose:
                    print(f'[Client-{self.id}] Error processing packet: {e}')
        return None

    def _wait_for_packet(self, expected_types, timeout):
        if isinstance(expected_types, int):  # PacketType is an IntEnum
            expected_types = [expected_types]
        elif not isinstance(expected_types, list):
            raise ValueError("expected_types must be a PacketType or a list of PacketTypes")

        end_time = time.time() + timeout
        while time.time() < end_time:
            packet = self._receive_packet(min(end_time - time.time(), 1.0))
            if packet and packet.header.packet_type in expected_types:
                return packet
        return None

    def _send_ack(self, packet):
        ack_packet = RTDEXPacket(
            header=self._create_header(PacketType.ACKNOWLEDGEMENT, Priority.HIGH, packet.header.packet_uid),
            acknowledgement=Acknowledgement(
                latency=int((time.time()*1e9-packet.header.timestamp)/1000)
            )
        )
        self._send_packet(ack_packet)

    def _join(self, authentication_token: int):
        """Join the RTDEX server and return the session token."""
        join_request = RTDEXPacket(
            header=self._create_header(PacketType.JOIN_REQUEST, Priority.HIGH),
            join_request=JoinRequest(
                id=self.id,
                namespace=self.namespace,
                authentication_token=authentication_token
            )
        )

        if not self._send_packet(join_request):
            raise Exception("Failed to join after maximum retries")

        # Wait for JOIN_RESPONSE or ERROR_MESSAGE
        response = self._wait_for_packet([PacketType.JOIN_RESPONSE, PacketType.ERROR_MESSAGE], self.response_timeout)
        if not response:
            raise Exception("No response received for JOIN_REQUEST")

        if response.header.packet_type == PacketType.ERROR_MESSAGE:
            error_code = ErrorCode.Name(response.error_message.error_code)
            raise Exception(f"Join failed with error: {error_code}")

        return response.join_response

    def _register_and_upload_data(self, name: str, data: bytes, freshness: int):
        data_size = len(data)
        num_chunks = -(-data_size // self.chunk_size)  # Ceiling division
        if self.verbose:
            print(
                f'[Client-{self.id}] Registering and uploading {name} - freshness: {freshness} seconds, size: {data_size}, chunks: {num_chunks}')

        for chunk_id in range(num_chunks):
            start = chunk_id * self.chunk_size
            end = start + self.chunk_size
            chunk = data[start:end]
            chunk_name = f"{name}" if num_chunks == 1 else f"{name}/{chunk_id + 1}"
            chunk_size = len(chunk)
            checksum = zlib.crc32(chunk)

            self._register_data(chunk_name, chunk, chunk_size, freshness, checksum)
            time.sleep(0.001)

    def _register_data(self, data_name: str, data: bytes, data_size: int, freshness: int, checksum: int):
        if self.verbose:
            print(
                f"[Client-{self.id}] Registering data: {data_name}, size: {data_size}, freshness: {freshness} seconds, checksum: {checksum:X}")

        register_packet = RTDEXPacket(
            header=self._create_header(PacketType.DATA_REGISTER, Priority.HIGH),
            data_register=DataRegister(
                name=data_name,
                freshness=freshness,
                size=data_size,
            )
        )

        if self._send_packet(register_packet):
            self._upload_data(data_name, data, checksum)
        else:
            raise Exception("Failed to register data after maximum retries")

    def _upload_data(self, data_name: str, data: bytes, checksum: int):
        if self.verbose:
            print(f"[Client-{self.id}] Uploading data: {data_name}, checksum: {checksum:X}")

        upload_packet = RTDEXPacket(
            header=self._create_header(PacketType.DATA_CONTENT, Priority.HIGH),
            data_content=DataContent(
                name=data_name,
                checksum=checksum,
                data=data,
            )
        )

        if not self._send_packet(upload_packet):
            raise Exception("Failed to upload data after maximum retries")


def main():
    api = RTDEX_API(3, "/api/test", verbose=True)

    try:
        if not api.connect(123):
            print(f"[Client-{api.id}] Failed to connect. Exiting.")
            return

        # Upload data
        data = b'Hello, RTDEX!'
        name = '/test/data'
        api.put(name, data, freshness=300)
        # Request data
        retrieved_data = api.get(name, timeout=5.0)
        if retrieved_data is not None:
            print(f"Retrieved data: {retrieved_data}")
        else:
            print("Failed to retrieve data")

    except KeyboardInterrupt:
        print(f"[Client-{api.id}] Client shutting down")
    finally:
        api.disconnect()


if __name__ == '__main__':
    main()
