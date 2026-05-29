package main

import (
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func startHTTPServer(serverCfg ServerConfig, metricsCfg MetricsConfig, replyCfg ReplyConfig, overlayCfg OverlayConfig, twitchClient twitchMessenger, defaultChannel, defaultUser string, chatLogger chatMessageLogger, overlay *chatOverlay) {
	mux := http.NewServeMux()
	mux.Handle(metricsCfg.Path, promhttp.Handler())
	log.Printf("Metrics laufen auf %s%s", serverCfg.Address, metricsCfg.Path)

	if replyCfg.Enabled {
		mux.HandleFunc(replyCfg.Path, handleReplyRequest(replyCfg, twitchClient, defaultChannel, defaultUser, chatLogger))
		log.Printf("n8n Rueckkanal laeuft auf %s%s", serverCfg.Address, replyCfg.Path)
		if replyCfg.Token == "" {
			log.Println("n8n Rueckkanal laeuft ohne Token-Schutz")
		}
	} else {
		log.Println("n8n Rueckkanal ist deaktiviert")
	}

	if overlayCfg.Enabled && overlay != nil {
		overlayPageHandler := overlay.handlePage(overlayCfg, defaultChannel)
		mux.HandleFunc(overlayCfg.Path, overlayPageHandler)
		mux.HandleFunc(overlayCfg.Path+"/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != overlayCfg.Path+"/" {
				http.NotFound(w, r)
				return
			}
			overlayPageHandler(w, r)
		})
		mux.HandleFunc(overlayCfg.eventPath(), overlay.handleEvents())
		log.Printf("OBS Chat-Overlay laeuft auf %s%s", serverCfg.Address, overlayCfg.Path)
	} else {
		log.Println("OBS Chat-Overlay ist deaktiviert")
	}

	server := &http.Server{
		Addr:              serverCfg.Address,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("HTTP-Server laeuft auf %s", serverCfg.Address)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("HTTP-Server konnte nicht gestartet werden: %v", err)
		}
	}()
}
