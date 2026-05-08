# Twitch IRC Exporter

Kleine Go-Anwendung, die Twitch-Chatnachrichten liest, an einen n8n-Webhook sendet und Prometheus-Metriken bereitstellt.

## Konfiguration

Die Anwendung liest standardmaessig `config.toml`. Eine andere Datei kann per Flag uebergeben werden:

```sh
go run . -config config.toml
```

Die wichtigsten Werte stehen in `config.example.toml`. `config.toml` ist in `.gitignore` eingetragen, weil dort der Twitch-OAuth-Token liegt.

## Metriken

Standardmaessig laufen die Prometheus-Metriken unter:

```text
http://localhost:2112/metrics
```
