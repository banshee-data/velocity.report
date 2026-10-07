package api

import (
	"net/http"

	"github.com/banshee-data/velocity.report/internal/access"
	"github.com/banshee-data/velocity.report/internal/db"
)

func canReadConfiguration(r *http.Request) bool {
	p, ok := requestPrincipal(r)
	return ok && p.Allows(access.Request{Operation: access.ReadConfiguration, Resource: r.URL.Path})
}

func (s *Server) siteResponse(r *http.Request, site *db.Site) any {
	if !s.hardened() || canReadConfiguration(r) {
		return site
	}
	// Whitelist display fields so later confidential DB fields cannot leak.
	return map[string]any{"id": site.ID, "name": site.Name, "location": site.Location,
		"latitude": site.Latitude, "longitude": site.Longitude, "map_angle": site.MapAngle,
		"include_map": site.IncludeMap, "created_at": site.CreatedAt, "updated_at": site.UpdatedAt}
}

func (s *Server) reportResponse(r *http.Request, report *db.SiteReport) any {
	if !s.hardened() {
		return report
	}
	out := map[string]any{"id": report.ID, "site_id": report.SiteID,
		"start_date": report.StartDate, "end_date": report.EndDate, "filename": report.Filename,
		"timezone": report.Timezone, "units": report.Units, "source": report.Source, "created_at": report.CreatedAt}
	p, ok := requestPrincipal(r)
	if ok && p.Allows(access.Request{Operation: access.ExportData, Resource: r.URL.Path}) {
		out["zip_filename"] = report.ZipFilename
	}
	return out
}

func (s *Server) reportListResponse(r *http.Request, reports []db.SiteReport) any {
	if !s.hardened() {
		return reports
	}
	out := make([]any, 0, len(reports))
	for i := range reports {
		out = append(out, s.reportResponse(r, &reports[i]))
	}
	return out
}
