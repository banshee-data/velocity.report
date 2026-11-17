// lidar-ego-track derives the moving sensor platform's speed, acceleration,
// jerk, yaw rate, stops and vibration from a trajectory file, with the
// uncertainty of each, and checks the trajectory's ends against known poses.
//
//	make build-lidar-ego-track
//	/tmp/lidar-ego-track -trajectory traj.json -anchors anchors.json -out-dir /local/ego/<drive>
//
// The trajectory is the operator's location history: keep inputs and outputs
// outside the checkout and never commit them.
package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/egotrack"
	"github.com/banshee-data/velocity.report/internal/version"
)

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("lidar-ego-track", flag.ContinueOnError)
	trajPath := fs.String("trajectory", "", "trajectory JSON ("+egotrack.SchemaV0+")")
	anchorPath := fs.String("anchors", "", "optional JSON array of end anchors in the trajectory's frame")
	outDir := fs.String("out-dir", "", "new or empty local output directory")
	var o egotrack.Options
	fs.Float64Var(&o.HalfWindowS, "half-window-s", 0, "local fit half-width (default 0.5)")
	fs.IntVar(&o.Degree, "degree", 0, "local fit degree (default 3)")
	fs.Float64Var(&o.DefaultSigmaPosM, "sigma-pos-m", 0, "per-axis pose noise when the trajectory states none (default 0.02)")
	fs.Float64Var(&o.SigmaYawRad, "sigma-yaw-rad", 0, "per-sample yaw noise (default 0.002)")
	fs.Float64Var(&o.StopSpeedMS, "stop-speed-ms", 0, "stop threshold (default 0.3)")
	fs.Float64Var(&o.StopMinS, "stop-min-s", 0, "minimum stop length (default 2)")
	fs.Float64Var(&o.SpectrumSegmentS, "spectrum-segment-s", 0, "Welch segment length in seconds (default 6.4)")
	fs.Float64Var(&o.AnchorWindowS, "anchor-window-s", 0, "seconds averaged at each end for anchors (default 5)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *trajPath == "" || *outDir == "" {
		return errors.New("trajectory and out-dir are required")
	}
	if err := o.Validate(); err != nil {
		return err
	}
	f, err := os.Open(*trajPath)
	if err != nil {
		return err
	}
	tr, err := egotrack.Read(f)
	f.Close()
	if err != nil {
		return err
	}
	var anchors []egotrack.Anchor
	if *anchorPath != "" {
		b, err := os.ReadFile(*anchorPath)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &anchors); err != nil {
			return fmt.Errorf("anchors: %w", err)
		}
	}
	sum, kin, err := egotrack.Analyse(tr, anchors, o)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	if ents, err := os.ReadDir(*outDir); err != nil || len(ents) > 0 {
		if err == nil {
			err = fmt.Errorf("output directory %s is not empty", *outDir)
		}
		return err
	}
	out := struct {
		Build      map[string]string `json:"build"`
		Trajectory map[string]string `json:"trajectory"`
		egotrack.Summary
	}{map[string]string{"version": version.Version, "git_sha": version.GitSHA}, map[string]string{"path": filepath.Base(*trajPath), "frame": tr.Frame, "time_scale": tr.TimeScale, "epoch_utc": tr.EpochUTC}, sum}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "summary.json"), append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := writeCSV(filepath.Join(*outDir, "kinematics.csv"), kin); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "samples=%d valid=%d duration=%.1fs distance=%.1fm max_speed=%.2fm/s stops=%d median_sigma speed=%.3f accel=%.3f jerk=%.3f jitter_peak pitch=%.2fHz/%.3fdeg z=%.2fHz/%.4fm\n",
		sum.Samples, sum.ValidSamples, sum.DurationS, sum.DistanceM, sum.MaxSpeedMS, len(sum.Stops),
		sum.MedianSpeedSigma, sum.MedianAccelSigma, sum.MedianJerkSigma, sum.Jitter.PeakPitchHz, sum.Jitter.PeakPitchAmpDeg, sum.Jitter.PeakZHz, sum.Jitter.PeakZAmpM)
	for _, a := range sum.Anchors {
		fmt.Fprintf(stdout, "anchor %s (%s, held_out=%t): horiz=%.3fm vert=%.3fm spread=%.3fm\n", a.Name, a.At, a.HeldOut, a.HorizErrM, a.VertErrM, a.SpreadM)
	}
	return nil
}

func writeCSV(path string, kin []egotrack.Kinematics) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	ff := func(v float64) string { return strconv.FormatFloat(v, 'g', 9, 64) }
	_ = w.Write([]string{"frame", "t_s", "valid", "reason", "speed_ms", "speed_sigma_ms", "heading_valid", "course_rad",
		"accel_long_ms2", "accel_lat_ms2", "accel_norm_ms2", "accel_sigma_ms2", "jerk_long_ms3", "jerk_norm_ms3", "jerk_sigma_ms3",
		"yaw_rate_rads", "yaw_rate_sigma_rads", "resid_z_m", "resid_pitch_rad", "resid_roll_rad"})
	for _, k := range kin {
		row := []string{strconv.Itoa(k.Frame), ff(k.T), strconv.FormatBool(k.Valid), k.Reason}
		if !k.Valid {
			row = append(row, make([]string, 16)...)
		} else {
			long, lat, jl, course := "", "", "", ""
			if k.HeadingValid {
				long, lat, jl, course = ff(k.AccelLong), ff(k.AccelLat), ff(k.JerkLong), ff(k.CourseRad)
			}
			row = append(row, ff(k.Speed), ff(k.SpeedSigma), strconv.FormatBool(k.HeadingValid), course, long, lat, ff(k.AccelNorm), ff(k.AccelSigma),
				jl, ff(k.JerkNorm), ff(k.JerkSigma), ff(k.YawRate), ff(k.YawRateSigma), ff(k.ResidZ), ff(k.ResidPitch), ff(k.ResidRoll))
		}
		if err := w.Write(row); err != nil {
			f.Close()
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
