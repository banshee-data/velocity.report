package api

// GET /api/charts/histogram?kind=headway&scene=<scene-id> renders a scene's
// headway distribution as SVG (D-11, D-17): the same source, version and
// distribution GET /api/scenes/<id>/headway serves, drawn by
// chart.RenderHeadwayHistogram. It takes that endpoint's selection
// parameters (source_id, stage and the four version axes), plus:
//
//	metric      a distributed metric id; default interaction.following_net_time_gap_s
//	paper_size  a4 or letter, as for the other charts
//
// Only kind=headway leaves the speed histogram's path, so every existing
// request is served exactly as before. A selection with nothing to draw is
// a labelled message chart, not an error; a malformed one is a JSON 400 and
// an unknown scene a JSON 404, as for the scene API.

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/report/chart"
)

// histogramKindHeadway selects the headway histogram on /api/charts/histogram.
const histogramKindHeadway = "headway"

func (s *Server) handleChartHeadwayHistogram(w http.ResponseWriter, r *http.Request, q url.Values) {
	sceneID := q.Get("scene")
	if !sceneIDPattern.MatchString(sceneID) {
		s.writeJSONError(w, http.StatusBadRequest, "'scene' is required with kind=headway: a scene id")
		return
	}
	metric := l8behaviour.MetricFollowingNetTimeGap
	if raw := q.Get("metric"); raw != "" {
		metric = l8behaviour.MetricID(raw)
		found := false
		for _, id := range l8behaviour.DistributedMetrics() {
			found = found || id == metric
		}
		if !found {
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid 'metric'. Must be one of: %v", l8behaviour.DistributedMetrics()))
			return
		}
	}
	resp, ok := s.loadSceneHeadway(w, r, sceneID, q)
	if !ok {
		return
	}

	style := chart.DefaultHeadwayHistogramStyle(parsePaperSize(q))
	var svg []byte
	var err error
	if resp.Distribution == nil {
		svg, err = chart.RenderHeadwayMessage(resp.Status.Label(), style, headwayMessage(resp, q))
	} else {
		svg, err = chart.RenderHeadwayHistogram(headwayChartData(*resp.Distribution, metric, resp.Status), style)
	}
	if err != nil {
		log.Printf("Chart headway render error for scene %s: %v", sceneID, err)
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to render chart")
		return
	}
	writeSVG(w, svg)
}

// headwayMessage says why a selection has nothing to draw.
func headwayMessage(resp sceneHeadwayResponse, q url.Values) string {
	switch resp.Availability {
	case headwayNoCaptureWindow:
		return "This scene has no capture window, so no following analysis can be matched to it."
	case headwayNoEncounters:
		return "No following encounters are stored for this scene's capture window."
	case headwaySourceAmbiguous:
		return fmt.Sprintf("%d analyses cover this capture window; choose one with source_id.", len(resp.Sources))
	default:
		stage := q.Get("stage")
		if stage == "" {
			stage = l8behaviour.StageFinal.String()
		}
		return fmt.Sprintf("No following analysis at stage %s matches this selection.", stage)
	}
}

// headwayChartData maps a distribution onto the chart's input, with every
// name from the registries: the metric id, the band thresholds and their
// benchmark kind, and the excluded reasons in precedence order.
func headwayChartData(d l8behaviour.FollowingDistribution, metric l8behaviour.MetricID,
	status l8behaviour.SurfaceStatus) chart.HeadwayHistogramData {
	h := d.Histograms[metric]
	data := chart.HeadwayHistogramData{
		StatusLabel: status.Label(), Metric: string(metric), Unit: h.Unit,
		AccountedNanos: d.AccountedNanos, RecordGapNanos: d.Accounting.RecordGapNanos,
		Encounters: d.Events, InBins: d.ExposureEvents,
	}
	for _, b := range h.Bins {
		data.Bins = append(data.Bins, chart.HeadwayBin{
			Lower: b.Lower, Upper: b.Upper, Nanos: b.Nanos,
			SigmaInsideNanos: b.SigmaInsideNanos, SigmaOverlapNanos: b.SigmaOverlapNanos,
		})
	}
	if metric == l8behaviour.MetricFollowingNetTimeGap {
		for _, band := range l8behaviour.FollowingBands() {
			data.Bands = append(data.Bands, chart.HeadwayBand{
				Threshold: band.Seconds, Label: strconv.FormatFloat(band.Seconds, 'f', 1, 64) + " s",
			})
			if def, ok := l8behaviour.LookupMetric(band.Duration); ok {
				data.BandBenchmark = def.Benchmark.String()
			}
		}
	}
	for _, r := range d.ExcludedReasons() {
		x := d.Excluded[r]
		data.Excluded = append(data.Excluded, chart.HeadwayExcluded{
			Reason: r.String(), Nanos: x.Nanos, PredictedOnlyNanos: x.PredictedOnlyNanos,
		})
	}
	return data
}
