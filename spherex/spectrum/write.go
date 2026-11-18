package spectrum

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// columns are the table's columns, each its unit after an underscore,
// as IRSA's spectrophotometry tool names them where it has the same.
var columns = []string{"wavelength_um", "bandwidth_um", "flux_ujy",
	"flux_err_ujy", "flux_err_stat_ujy", "flux_bkg_mjysr", "chi2_dof",
	"used", "fit_ql", "flags", "neighbours", "mjd", "det_id", "x_image",
	"y_image", "omega_arcsec2", "file"}

// Write writes points as a table: a line of the columns after a #,
// then a point a line, the wavelength rising, each value tab apart.
func Write(out io.Writer, points []Point) error {
	sorted := append([]Point(nil), points...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Wavelength < sorted[j].Wavelength
	})
	if _, err := fmt.Fprintf(out, "# %s\n", strings.Join(columns,
		"\t")); err != nil {
		return err
	}
	for _, point := range sorted {
		if _, err := fmt.Fprintln(out, point.line()); err != nil {
			return err
		}
	}
	return nil
}

// line is a point's values, in the columns' order.
func (point Point) line() string {
	return fmt.Sprintf("%.5f\t%.5f\t%.1f\t%.1f\t%.1f\t%.4f\t%.3f\t%d\t"+
		"%.3f\t%#x\t%d\t%.5f\t%d\t%.3f\t%.3f\t%.3f\t%s",
		point.Wavelength, point.Bandwidth, point.Flux, point.Error,
		point.Statistical, point.Background, point.ChiSquared,
		point.Used, point.Quality, point.Flags, point.Neighbours,
		point.MJD, point.Detector, point.X, point.Y, point.SolidAngle,
		point.File)
}
