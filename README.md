# Real-Time Data Exchange (RTDEX)

A real-time data exchange protocol and library inspired by Named-Data Networking (NDN) and IEEE 802.1Qbv Time-aware Shaper (TAS).

## Features

- Hierarchical name-based data access
- In-network data caching and async data transfer
- Time-aware priority queues for bounded latency
- Protocol Buffer integration
- Flexible session management and housekeeping
- Data chunking and checksum

## Architecture

```mermaid
graph BT
    subgraph Server[Engine]
        S_UDPSocket[UDP Socket]
        S_ProtoBuf[Protocol Buffer Encoding/Decoding]
        S_Cache[In-Network Cache]

        subgraph S_SessionMgr[Session Manager]
            subgraph S_Session[Client Session i]
                S_PriorityQueues[Priority Queues]
                S_TimeAwareScheduler[Time-Aware Scheduler]
            end
        end

        S_SlotMgr[Slot Manager]
        S_WebServer[Web Server]
        S_WebUI[Web UI]
    end

    subgraph Publisher[Publisher client]
        P_UDPSocket[UDP Socket]
        P_ProtoBuf[Protocol Buffer Encoding/Decoding]
        P_DataProcessing[Data Processing]
        P_PublishLogic[Publish Logic]
    end

    subgraph Subscriber[Subscriber client]
        Sub_UDPSocket[UDP Socket]
        Sub_ProtoBuf[Protocol Buffer Encoding/Decoding]
        Sub_DataProcessing[Data Processing]
        Sub_SubscribeLogic[Subscribe Logic]
    end

    %% Connections
    P_PublishLogic --> P_DataProcessing
    P_DataProcessing <--> P_ProtoBuf
    P_ProtoBuf <--> P_UDPSocket
    P_UDPSocket <-->|UDP| S_UDPSocket

    Sub_SubscribeLogic --> Sub_DataProcessing
    Sub_DataProcessing <--> Sub_ProtoBuf
    Sub_ProtoBuf <--> Sub_UDPSocket
    Sub_UDPSocket <-->|UDP| S_UDPSocket

    S_UDPSocket <--> S_ProtoBuf
    S_ProtoBuf <--> S_Session
    S_TimeAwareScheduler --> S_PriorityQueues
    S_SlotMgr --> S_TimeAwareScheduler

    S_ProtoBuf <--> S_Cache

    S_Cache --> S_WebServer
    S_SessionMgr --> S_WebServer

    S_WebServer <-- WebSocket --> S_WebUI
```

## Usage

```go
engine := engine.NewEngine(config.DefaultConfig)
engine.Start()

client := client.NewClient(3, "device-type-1", config.DefaultConfig)
client.Connect()
client.Put("/data/test", []byte("test"), 60)

data, err := client.Get("/data/test", time.Second*10)

if err != nil {
    fmt.Println(err)
}
fmt.Println("data:", string(data))

client.Disconnect()

engine.Stop()
```

![packets](./screenshot-packets.png)
![cache](./screenshot-data.png)
