import time
from client import Client


def main():
    client = Client(3)

    try:
        if not client.connect(123):
            print(f"[Client-{client.id}] Failed to connect. Exiting.")
            return

        # Upload data
        with open('random_file.bin', 'rb') as f:
            data = f.read()
        name = '/data/test/random_file.bin'
        client.put(name, data, freshness=300)
        time.sleep(1)

        # Request data
        retrieved_data = client.get(name, timeout=5.0)
        if retrieved_data is not None:
            print(f"Retrieved data size: {len(retrieved_data)}")
        else:
            print("Failed to retrieve data")

    except KeyboardInterrupt:
        print(f"[Client-{client.id}] Client shutting down")
    finally:
        client.disconnect()


if __name__ == '__main__':
    main()
