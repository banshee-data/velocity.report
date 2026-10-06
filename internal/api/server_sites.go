package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/db"
)

// handleSites routes site-related requests to appropriate handlers
func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Parse the path to extract ID if present
	// URL format: /api/sites or /api/sites/123
	path := strings.TrimPrefix(r.URL.Path, "/api/sites")
	path = strings.Trim(path, "/")

	// List or Create
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			s.listSites(w, r)
		case http.MethodPost:
			s.createSite(w, r)
		default:
			s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}

	// Get, Update, or Delete by ID
	id, err := strconv.Atoi(path)
	if err != nil {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid site ID")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getSite(w, r, id)
	case http.MethodPut:
		s.updateSite(w, r, id)
	case http.MethodDelete:
		s.deleteSite(w, r, id)
	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listSites(w http.ResponseWriter, r *http.Request) {
	sites, err := s.db.GetAllSites(r.Context())
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve sites: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(sites); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode sites")
		return
	}
}

func (s *Server) getSite(w http.ResponseWriter, r *http.Request, id int) {
	site, err := s.db.GetSite(r.Context(), id)
	if err != nil {
		if err.Error() == "site not found" {
			s.writeJSONError(w, http.StatusNotFound, "Site not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve site: %v", err))
		}
		return
	}

	if err := json.NewEncoder(w).Encode(site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site")
		return
	}
}

func (s *Server) createSite(w http.ResponseWriter, r *http.Request) {
	var site db.Site
	if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	// Validate required fields
	if site.Name == "" {
		s.writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if site.Location == "" {
		s.writeJSONError(w, http.StatusBadRequest, "location is required")
		return
	}

	if err := s.db.CreateSite(r.Context(), &site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create site: %v", err))
		return
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site")
		return
	}
}

func (s *Server) updateSite(w http.ResponseWriter, r *http.Request, id int) {
	var site db.Site
	if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	site.ID = id

	// Validate required fields
	if site.Name == "" {
		s.writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	if site.Location == "" {
		s.writeJSONError(w, http.StatusBadRequest, "location is required")
		return
	}

	if err := s.db.UpdateSite(r.Context(), &site); err != nil {
		if err.Error() == "site not found" {
			s.writeJSONError(w, http.StatusNotFound, "Site not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update site: %v", err))
		}
		return
	}

	if err := json.NewEncoder(w).Encode(site); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site")
		return
	}
}

func (s *Server) deleteSite(w http.ResponseWriter, r *http.Request, id int) {
	if err := s.db.DeleteSite(r.Context(), id); err != nil {
		if err.Error() == "site not found" {
			s.writeJSONError(w, http.StatusNotFound, "Site not found")
		} else {
			s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to delete site: %v", err))
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSiteConfigPeriods(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		s.listSiteConfigPeriods(w, r)
	case http.MethodPost:
		s.upsertSiteConfigPeriod(w, r)
	default:
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) listSiteConfigPeriods(w http.ResponseWriter, r *http.Request) {
	var siteID *int
	if siteIDStr := r.URL.Query().Get("site_id"); siteIDStr != "" {
		parsedSiteID, err := strconv.Atoi(siteIDStr)
		if err != nil || parsedSiteID <= 0 {
			s.writeJSONError(w, http.StatusBadRequest, "Invalid 'site_id' parameter; must be a positive integer")
			return
		}
		siteID = &parsedSiteID
	}

	periods, err := s.db.ListSiteConfigPeriods(siteID)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve site config periods: %v", err))
		return
	}

	if err := json.NewEncoder(w).Encode(periods); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site config periods")
		return
	}
}

func (s *Server) upsertSiteConfigPeriod(w http.ResponseWriter, r *http.Request) {
	var period db.SiteConfigPeriod
	if err := json.NewDecoder(r.Body).Decode(&period); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid JSON: %v", err))
		return
	}

	if period.SiteID == 0 {
		s.writeJSONError(w, http.StatusBadRequest, "site_id is required")
		return
	}

	if period.ID > 0 {
		if err := s.db.UpdateSiteConfigPeriod(&period); err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "site config period not found" || err.Error() == "site config period overlaps an existing period" {
				status = http.StatusBadRequest
			}
			s.writeJSONError(w, status, fmt.Sprintf("Failed to update site config period: %v", err))
			return
		}
		if err := json.NewEncoder(w).Encode(period); err != nil {
			s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site config period")
		}
		return
	}

	if err := s.db.CreateSiteConfigPeriod(&period); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Failed to create site config period: %v", err))
		return
	}

	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(period); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode site config period")
	}
}
