package mapper

import (
	"log"
	"net/http"
	"sort"

	netboxapi "github.com/netbox-community/go-netbox/v4"
)

// indexData is the template context for the index page
type indexData struct {
	Rows       []SiteRow
	NetBoxSites []netboxapi.Site
	Regions     []netboxapi.Region
	SiteGroups  []netboxapi.SiteGroup
	Statuses    []string
	MappedCount int
	TotalCount  int
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Fetch Mist sites
	mistSites, err := s.mist.GetSites()
	if err != nil {
		http.Error(w, "Failed to fetch Mist sites: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Fetch NetBox data
	nbSites, err := s.netbox.ListSites()
	if err != nil {
		http.Error(w, "Failed to fetch NetBox sites: "+err.Error(), http.StatusInternalServerError)
		return
	}
	regions, err := s.netbox.ListRegions()
	if err != nil {
		http.Error(w, "Failed to fetch NetBox regions: "+err.Error(), http.StatusInternalServerError)
		return
	}
	siteGroups, err := s.netbox.ListSiteGroups()
	if err != nil {
		http.Error(w, "Failed to fetch NetBox site groups: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build rows from Mist sites, merging existing mappings
	s.mu.RLock()
	rows := make([]SiteRow, 0, len(mistSites))
	for _, ms := range mistSites {
		row := SiteRow{
			MistSiteID:   ms.SourceID,
			MistSiteName: ms.Name,
			MistAddress:  ms.Address,
		}
		if existing, ok := s.mappings[ms.SourceID]; ok {
			row.NetBoxSite = existing.NetBoxSite
			row.Region = existing.Region
			row.SiteGroup = existing.SiteGroup
			row.Status = existing.Status
			row.Saved = existing.Saved
		}
		rows = append(rows, row)
	}
	s.mu.RUnlock()

	// Sort: unmapped first, then alphabetical
	sort.Slice(rows, func(i, j int) bool {
		iMapped := rows[i].NetBoxSite != ""
		jMapped := rows[j].NetBoxSite != ""
		if iMapped != jMapped {
			return !iMapped
		}
		return rows[i].MistSiteName < rows[j].MistSiteName
	})

	mappedCount := 0
	for _, row := range rows {
		if row.NetBoxSite != "" {
			mappedCount++
		}
	}

	data := indexData{
		Rows:        rows,
		NetBoxSites: nbSites,
		Regions:     regions,
		SiteGroups:  siteGroups,
		Statuses:    []string{"active", "planned", "staging", "retired"},
		MappedCount: mappedCount,
		TotalCount:  len(rows),
	}

	if err := s.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Template rendering error", http.StatusInternalServerError)
	}
}

func (s *Server) handleSaveMapping(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	mistID := r.FormValue("mist_site_id")
	if mistID == "" {
		http.Error(w, "Missing mist_site_id", http.StatusBadRequest)
		return
	}

	row := &SiteRow{
		MistSiteID:   mistID,
		MistSiteName: r.FormValue("mist_site_name"),
		MistAddress:  r.FormValue("mist_address"),
		NetBoxSite:   r.FormValue("netbox_site"),
		Region:       r.FormValue("region"),
		SiteGroup:    r.FormValue("site_group"),
		Status:       r.FormValue("status"),
		Saved:        true,
	}

	s.mu.Lock()
	s.mappings[mistID] = row
	s.mu.Unlock()

	// Fetch dropdown data for the partial re-render
	nbSites, _ := s.netbox.ListSites()
	regions, _ := s.netbox.ListRegions()
	siteGroups, _ := s.netbox.ListSiteGroups()

	partialData := struct {
		Row        SiteRow
		NetBoxSites []netboxapi.Site
		Regions     []netboxapi.Region
		SiteGroups  []netboxapi.SiteGroup
		Statuses    []string
	}{
		Row:         *row,
		NetBoxSites: nbSites,
		Regions:     regions,
		SiteGroups:  siteGroups,
		Statuses:    []string{"active", "planned", "staging", "retired"},
	}

	if err := s.templates.ExecuteTemplate(w, "site_row.html", partialData); err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Template rendering error", http.StatusInternalServerError)
	}
}

func (s *Server) handleBulkSave(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	mistIDs := r.Form["mist_site_id"]
	mistNames := r.Form["mist_site_name"]
	mistAddresses := r.Form["mist_address"]
	nbSites := r.Form["netbox_site"]
	regionVals := r.Form["region"]
	sgVals := r.Form["site_group"]
	statusVals := r.Form["status"]

	s.mu.Lock()
	for i, id := range mistIDs {
		nbSite := ""
		if i < len(nbSites) {
			nbSite = nbSites[i]
		}
		region := ""
		if i < len(regionVals) {
			region = regionVals[i]
		}
		sg := ""
		if i < len(sgVals) {
			sg = sgVals[i]
		}
		status := ""
		if i < len(statusVals) {
			status = statusVals[i]
		}

		name := ""
		if i < len(mistNames) {
			name = mistNames[i]
		}
		addr := ""
		if i < len(mistAddresses) {
			addr = mistAddresses[i]
		}

		s.mappings[id] = &SiteRow{
			MistSiteID:   id,
			MistSiteName: name,
			MistAddress:  addr,
			NetBoxSite:   nbSite,
			Region:       region,
			SiteGroup:    sg,
			Status:       status,
			Saved:        true,
		}
	}
	s.mu.Unlock()

	// Redirect back to index
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(http.StatusOK)
}
