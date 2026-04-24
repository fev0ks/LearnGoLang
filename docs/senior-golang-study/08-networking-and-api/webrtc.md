# WebRTC

WebRTC (Web Real-Time Communication) — браузерный стандарт P2P медиа и данных без сервера-посредника. Видеозвонки, аудиоконференции, peer-to-peer файлообмен.

---

## Peer-to-peer: signaling, ICE, STUN/TURN

### Парадокс P2P: как найти друг друга без сервера

P2P соединение без посредника — но чтобы установить прямое соединение, нужно обменяться адресами. Для этого нужен **signaling сервер** (обычно WebSocket).

```
Alice                Signaling Server            Bob
  │                       │                       │
  │── offer (SDP) ───────►│──── offer ───────────►│
  │                       │                       │ (создаёт answer)
  │◄── answer (SDP) ──────│◄─── answer ───────────│
  │                       │                       │
  │── ICE candidate ──────►│── ICE candidate ─────►│
  │◄──────────────────────│◄──────────────────────│
  │                       │                       │
  │◄══════════════════════════════════════════════►│
  │                 P2P соединение                 │
  │         (signaling сервер больше не нужен)     │
```

### SDP — Session Description Protocol

SDP описывает возможности участника: кодеки, форматы, IP адреса.

```
v=0
o=alice 2890844526 2890844526 IN IP4 alice.example.com
s=
c=IN IP4 192.168.1.1
t=0 0
m=audio 49170 RTP/AVP 0
a=rtpmap:0 PCMU/8000
m=video 51372 RTP/AVP 31
a=rtpmap:31 H261/90000
```

```
Offer/Answer механизм:
1. Alice создаёт offer (что я умею)
2. Bob получает offer → создаёт answer (что я умею и что я принимаю)
3. Alice получает answer → соединение установлено
```

### ICE — Interactive Connectivity Establishment

ICE находит лучший путь для P2P соединения:

```
ICE candidate types:
  host:        192.168.1.100:50000     ← прямое LAN соединение (лучший)
  srflx:       203.0.113.1:50001      ← через NAT (STUN помогает найти)
  relay:       203.0.113.100:3478     ← через TURN сервер (худший)
```

### STUN — Session Traversal Utilities for NAT

STUN сервер рассказывает клиенту его **публичный IP** (за NAT):

```
Client (192.168.1.1) ──STUN request──► STUN Server
                     ◄──your IP: 203.0.113.1:50000──
```

### TURN — Traversal Using Relays around NAT

Когда STUN не помогает (symmetric NAT, firewall) — TURN ретранслирует трафик:

```
Alice ──────► TURN Server ──────► Bob
(трафик идёт через сервер — дорого, но всегда работает)
```

**Правило**: всегда иметь TURN fallback. P2P через STUN — ~80% случаев. TURN нужен для оставшихся 20%.

---

## WebRTC в Go: Pion

[Pion](https://github.com/pion/webrtc) — реализация WebRTC на Go.

```go
import "github.com/pion/webrtc/v4"

// Создать PeerConnection
config := webrtc.Configuration{
    ICEServers: []webrtc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}}, // бесплатный STUN
        {
            URLs:       []string{"turn:turn.example.com:3478"},
            Username:   "user",
            Credential: "password",
        },
    },
}

pc, err := webrtc.NewPeerConnection(config)
if err != nil {
    log.Fatal(err)
}
defer pc.Close()

// Добавить видео трек
videoTrack, err := webrtc.NewTrackLocalStaticSample(
    webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8},
    "video", "camera",
)
_, err = pc.AddTrack(videoTrack)

// Обработать входящие треки
pc.OnTrack(func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
    fmt.Printf("Got track: %s %s\n", track.Kind(), track.Codec().MimeType)
    for {
        rtpPacket, _, err := track.ReadRTP()
        if err != nil { return }
        // обработать RTP пакет (decode video/audio)
        _ = rtpPacket
    }
})

// ICE candidate handler
pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
    if candidate != nil {
        // Отправить через signaling
        signalingServer.SendICECandidate(candidate.ToJSON())
    }
})

// Создать offer
offer, err := pc.CreateOffer(nil)
pc.SetLocalDescription(offer)

// Отправить offer через signaling
signalingServer.SendOffer(offer)

// Получить answer от signaling
answer := signalingServer.ReceiveAnswer()
pc.SetRemoteDescription(answer)

// Получать ICE candidates от peer и добавлять
candidate := signalingServer.ReceiveICECandidate()
pc.AddICECandidate(candidate)
```

---

## WebRTC vs WebSocket

| | WebRTC | WebSocket |
|---|---|---|
| Архитектура | P2P (прямое соединение) | Client-Server |
| Медиа (video/audio) | ✅ нативно (RTP) | ❌ (нужен media server) |
| Signaling | Нужен отдельный (WS) | Не нужен |
| Latency | Очень низкая (< 100ms) | Низкая (< 10ms) |
| NAT traversal | Встроено (ICE/STUN/TURN) | Не нужно |
| Data channel | ✅ DataChannel API | ✅ |
| Scalability | Трудно (P2P mesh) | Легко (сервер централен) |
| Сложность | Высокая | Средняя |

**WebRTC для**: видеозвонки, live streaming P2P, screensharing, file transfer P2P.

**WebSocket для**: чат, live updates, collaboration (all hub-and-spoke, не P2P).

---

## Масштабирование WebRTC

P2P mesh — проблема при N > 2-3 участников:

```
4 участника = 6 P2P соединений
N участников = N*(N-1)/2 соединений
```

**SFU (Selective Forwarding Unit)** — сервер-посредник для медиа:

```
Alice ──► SFU ──► Bob, Charlie, Dave
Bob   ──► SFU ──► Alice, Charlie, Dave
```

Каждый участник отправляет одному SFU, SFU форвардит. Популярные: Janus, mediasoup, Livekit.

---

## Interview-ready answer

**Q: Как работает WebRTC?**

WebRTC устанавливает P2P соединение через три этапа: signaling (обмен SDP offer/answer через сервер-посредник — обычно WebSocket), ICE (нахождение лучшего пути: прямой LAN, через NAT с помощью STUN, или relay через TURN), установка зашифрованного DTLS соединения. После этого медиа (RTP) идёт напрямую между браузерами. Signaling сервер больше не нужен.

**Q: Зачем STUN и TURN?**

STUN помогает клиентам за NAT узнать свой публичный IP — нужен для P2P через NAT. TURN — relay сервер когда STUN не помогает (symmetric NAT, строгий firewall). TURN всегда нужен как fallback — около 20% соединений не могут обойтись STUN.
