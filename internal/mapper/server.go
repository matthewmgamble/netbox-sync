package mapper

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"sync"

	"github.com/matthewmgamble/netbox-sync/internal/client/mist"
	"github.com/matthewmgamble/netbox-sync/internal/client/netbox"
	"github.com/matthewmgamble/netbox-sync/internal/config"
	"github.com/matthewmgamble/netbox-sync/internal/models"
)

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// SiteRow represents a row in the mapping table
type SiteRow struct {
	MistSiteID   string
	MistSiteName string
	MistAddress  string
	NetBoxSite   string
	Region       string
	SiteGroup    string
	Status       string
	Saved        bool
}

// Server is the mapper web UI server
type Server struct {
	mist        *mist.Client
	netbox      *netbox.Client
	mappingPath string
	listenAddr  string
	templates   *template.Template
	mux         *http.ServeMux

	mu       sync.RWMutex
	mappings map[string]*SiteRow // keyed by Mist site ID
	mapping  *models.MappingConfig
}

// NewServer creates a new mapper server
func NewServer(mistClient *mist.Client, netboxClient *netbox.Client, mappingPath string, listenAddr string) (*Server, error) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"eq": func(a, b string) bool { return a == b },
		"dict": func(pairs ...interface{}) map[string]interface{} {
			m := make(map[string]interface{}, len(pairs)/2)
			for i := 0; i < len(pairs)-1; i += 2 {
				m[pairs[i].(string)] = pairs[i+1]
			}
			return m
		},
	}).ParseFS(templateFS, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		return nil, err
	}

	// Load existing mapping if available
	var mappingCfg *models.MappingConfig
	if mappingPath != "" {
		mappingCfg, err = config.LoadMapping(mappingPath)
		if err != nil {
			// Non-fatal — start with empty mapping
			mappingCfg = &models.MappingConfig{}
		}
	} else {
		mappingCfg = &models.MappingConfig{}
	}

	s := &Server{
		mist:        mistClient,
		netbox:      netboxClient,
		mappingPath: mappingPath,
		listenAddr:  listenAddr,
		templates:   tmpl,
		mux:         http.NewServeMux(),
		mappings:    make(map[string]*SiteRow),
		mapping:     mappingCfg,
	}

	// Pre-populate mappings from existing config
	for mistID, siteMap := range mappingCfg.Sites {
		s.mappings[mistID] = &SiteRow{
			MistSiteID: mistID,
			NetBoxSite: siteMap.NetBoxSiteName,
			Region:     siteMap.Region,
			SiteGroup:  siteMap.SiteGroup,
			Status:     siteMap.Status,
		}
	}

	// Register routes
	staticSub, _ := fs.Sub(staticFS, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
	s.mux.HandleFunc("GET /", s.handleIndex)
	s.mux.HandleFunc("POST /api/mapping", s.handleSaveMapping)
	s.mux.HandleFunc("POST /api/mapping/bulk", s.handleBulkSave)
	s.mux.HandleFunc("GET /api/export", s.handleExport)

	return s, nil
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(s.listenAddr, s.mux)
}
