# Twitch IRC Exporter

Kleine Go-Anwendung, die Twitch-Chatnachrichten liest, an einen n8n-Webhook sendet, Prometheus-Metriken bereitstellt und empfangene sowie gesendete Chatnachrichten optional an Loki uebermittelt.

## Konfiguration

Die Anwendung liest standardmaessig `config.toml`. Eine andere Datei kann per Flag uebergeben werden:

```sh
go run . -config config.toml
```

Die wichtigsten Werte stehen in `config.example.toml`. `config.toml` ist in `.gitignore` eingetragen, weil dort der Twitch-OAuth-Token liegt.

## Docker

Vor dem Start die lokale Konfiguration anlegen:

```sh
cp config.example.toml config.toml
```

Wenn n8n auf dem Docker-Host laeuft, in `config.toml` fuer `n8n.url` statt `localhost` den Hostnamen `host.docker.internal` verwenden.
Wenn Loki auf dem Docker-Host laeuft, gilt dasselbe fuer `loki.url`.

Start per Compose:

```sh
docker compose up -d --build
```

## Metriken

Standardmaessig laeuft ein HTTP-Server auf `:2112`. Die Prometheus-Metriken sind dort unter:

```text
http://localhost:2112/metrics
```

## Loki

Loki kann in `config.toml` aktiviert werden:

```toml
[loki]
enabled = true
url = "http://localhost:3100/loki/api/v1/push"
timeout = "2s"

[loki.labels]
job = "twitch-irc"
```

Die Anwendung sendet empfangene Twitch-Nachrichten mit `direction="received"` und Nachrichten aus dem n8n-Rueckkanal mit `direction="sent"`. `channel` wird ebenfalls als Label gesetzt; User, Message-ID und Nachrichtentext stehen in der JSON-Logzeile.

## n8n Rueckkanal

n8n kann ueber den konfigurierten Rueckkanal Nachrichten zurueck in den Twitch-Chat schicken:

```sh
curl -X POST http://127.0.0.1:2112/n8n/reply \
  -H "Authorization: Bearer DEIN_RUECKKANAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Hallo Chat!"}'
```

Optional kann n8n `channel`, `user` und `reply_to_message_id` mitsenden. Ohne `channel` wird der konfigurierte Twitch-Channel verwendet, ohne `user` der konfigurierte Twitch-Username.

## OBS Chat-Overlay

Der Bot stellt ein transparentes Browser-Overlay fuer OBS bereit. Standard-URL:

```text
http://localhost:2112/overlay/chat
```

Das Overlay nutzt Server-Sent Events unter `/overlay/chat/events`, zeigt beim Laden noch aktive Nachrichten erneut an und blendet sie nach `overlay.message_ttl` aus. Nachrichten aus dem n8n-Rueckkanal erscheinen mit einem kleinen `Bot`-Hinweis.

HTML und CSS liegen getrennt unter `overlay_assets/chat.html` und `overlay_assets/chat.css`. Sie werden beim Build in das Go-Binary eingebettet.

Die Route kann fuer einen spaeteren Reverse-Proxy angepasst werden:

```toml
[overlay]
enabled = true
path = "/overlay/chat"
max_messages = 60
message_ttl = "45s"
```

Wichtig fuer den Proxy: Die Event-Route liegt relativ zur Overlay-URL unter `events`, also zum Beispiel `/overlay/chat/events`. Fuer Nginx sollte Streaming-Buffering fuer diese Route deaktiviert werden.

### Traefik

`docker-compose.yaml` veroeffentlicht nur das Overlay ueber Traefik:

- `/overlay/chat`
- `/overlay/chat/`
- `/overlay/chat/events`

Setze fuer die Internet-Route `TWITCH_IRC_OVERLAY_DOMAIN` auf die gewuenschte Domain. Wenn dieselbe Domain wie fuer n8n genutzt wird, bleibt n8n fuer alle anderen Pfade erreichbar; `/n8n/reply` wird nicht ueber Traefik an `twitch-irc` geroutet.
