# Twitch IRC Exporter

Kleine Go-Anwendung, die Twitch-Chatnachrichten liest, an einen n8n-Webhook sendet und Prometheus-Metriken bereitstellt.

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

Start per Compose:

```sh
docker compose up -d --build
```

## Metriken

Standardmaessig laeuft ein HTTP-Server auf `:2112`. Die Prometheus-Metriken sind dort unter:

```text
http://localhost:2112/metrics
```

## n8n Rueckkanal

n8n kann ueber den konfigurierten Rueckkanal Nachrichten zurueck in den Twitch-Chat schicken:

```sh
curl -X POST http://127.0.0.1:2112/n8n/reply \
  -H "Authorization: Bearer DEIN_RUECKKANAL_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Hallo Chat!"}'
```

Optional kann n8n `channel` und `reply_to_message_id` mitsenden. Ohne `channel` wird der konfigurierte Twitch-Channel verwendet.
