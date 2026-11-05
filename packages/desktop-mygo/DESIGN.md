# TokenInsights MyGo

The shared dashboard owns layout, theme and behavior. Default window 1280×850,
minimum 360×480. Connection failures use native dialogs.
Each window owns its requested backend session and private loopback bridge.
Desktop launch links contain an opaque rendezvous ID; native processes exchange
the connection and capture policy over a user-private Unix socket.
Pages receive browser capabilities only. No privileged page bindings.
