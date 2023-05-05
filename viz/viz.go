// Package viz contains code for displaying data visualizations.
package viz

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

type DataEntry struct {
	Name  string
	Date  int
	Value float64
}

var breakpointWidth = `
  function currentBreakpoint() {
    const width = window.innerWidth;
    if (width < 640) return "";
    if (width < 768) return "sm";
    if (width < 1024) return "md";
    if (width < 1280) return "lg";
    if (width < 1536) return "xl";
    return "2xl";
  }

  let baseWidth = 347;
  switch (currentBreakpoint()) {
    case "md":
      baseWidth = 600;
      break;
    case "lg":
      baseWidth = 392;
      break;
    case "xl":
      baseWidth = 452;
      break;
    case "2xl":
      baseWidth = 514;
      break;
  }
`

func fixedWidth(width int) string {
	return fmt.Sprintf(`
  const baseWidth = %d;
`, width)
}

func fixedHeight(height int) string {
	return fmt.Sprintf(`
  const baseHeight = %d;
`, height)
}

type ChartOptions struct {
	BaseWidthJS  string
	BaseHeightJS string
}

type ChartOption func(*ChartOptions)

func WithFixedHeight(height int) ChartOption {
	return func(o *ChartOptions) {
		o.BaseHeightJS = fixedHeight(height)
	}
}

func WithFixedWidth(width int) ChartOption {
	return func(o *ChartOptions) {
		o.BaseWidthJS = fixedWidth(width)
	}
}

func WithBreakpointWidth() ChartOption {
	return func(o *ChartOptions) {
		o.BaseWidthJS = breakpointWidth
	}
}

func GenerateBarChartJS(selector, title string, data []*DataEntry, isCurrency bool, options ...ChartOption) (string, error) {
	// Set default options
	opts := ChartOptions{
		BaseWidthJS:  breakpointWidth,
		BaseHeightJS: fixedHeight(750),
	}
	// Apply user-provided options
	for _, option := range options {
		option(&opts)
	}

	input := struct {
		Selector, Title string
		Data            []*DataEntry
		IsCurrency      bool
	}{
		Selector:   selector,
		Title:      title,
		Data:       data,
		IsCurrency: isCurrency,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshalling data: %v", err)
	}
	jsIntro := fmt.Sprintf(
		"%s\n%s\nconst input = %s;\n",
		opts.BaseWidthJS,
		opts.BaseHeightJS,
		string(inputJSON))
	jsFile, err := os.ReadFile("./viz/bar_chart.js")
	if err != nil {
		return "", fmt.Errorf("reading js file: %v", err)
	}
	return jsIntro + string(jsFile), nil
}

func GenerateLineChartJS(selector, title string, data []*DataEntry, isCurrency bool, options ...ChartOption) (string, error) {
	// Set default options
	opts := ChartOptions{
		BaseWidthJS: breakpointWidth,
	}
	// Apply user-provided options
	for _, option := range options {
		option(&opts)
	}

	input := struct {
		Selector, Title string
		Data            []*DataEntry
		IsCurrency      bool
	}{
		Selector:   selector,
		Title:      title,
		Data:       data,
		IsCurrency: isCurrency,
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("marshalling data: %v", err)
	}
	jsIntro := fmt.Sprintf(
		"%s\n%s\nconst input = %s;\n",
		opts.BaseWidthJS,
		opts.BaseHeightJS,
		string(inputJSON))
	jsFile, err := os.ReadFile("./viz/line_chart.js")
	if err != nil {
		return "", fmt.Errorf("reading js file: %v", err)
	}
	return jsIntro + string(jsFile), nil
}

const htmlContent = `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>SVG Export</title>

	<link href="https://torontoverse.com/css/style.css?v=7" rel="stylesheet" />

    <!-- fonts -->
    <link rel="preconnect" href="https://fonts.googleapis.com" />
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
    <link
      href="https://fonts.googleapis.com/css2?family=JetBrains+Mono&display=swap"
      rel="stylesheet"
    />
	<script src="https://torontoverse.com/js/lib/d3/d3.min.js"></script>
</head>
<body class="bg-map-50 font-mono">
    <script type="text/javascript">
    // Add your D3.js visualization code here
	  REPLACE_ME_WITH_CHART_JS
    </script>
</body>
</html>
`

// GenerateBarChartHTML generates an bare HTML file containing only styles, fonts and.
func GenerateBarChartHTML(title string, data []*DataEntry, isCurrency bool, options ...ChartOption) (string, error) {
	js, err := GenerateBarChartJS("body", title, data, isCurrency, options...)
	if err != nil {
		return "", fmt.Errorf("generating js: %v", err)
	}
	return strings.Replace(htmlContent, "REPLACE_ME_WITH_CHART_JS", js, 1), nil
}

// GenerateLineChartHTML generates an bare HTML file containing only styles, fonts and.
func GenerateLineChartHTML(title string, data []*DataEntry, isCurrency bool, options ...ChartOption) (string, error) {
	js, err := GenerateLineChartJS("body", title, data, isCurrency, options...)
	if err != nil {
		return "", fmt.Errorf("generating js: %v", err)
	}
	return strings.Replace(htmlContent, "REPLACE_ME_WITH_CHART_JS", js, 1), nil
}

func SVGToPNG(ctx context.Context, width, height float64, svgHTML string) ([]byte, error) {
	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 75*time.Second)
	defer cancel()

	var buf []byte
	if err := chromedp.Run(ctx, saveSVGAsPNG(svgHTML, width, height, &buf)); err != nil {
		return []byte{}, fmt.Errorf("running chromedp: %v", err)
	}
	return buf, nil
}

func saveSVGAsPNG(htmlContent string, width, height float64, buf *[]byte) chromedp.Tasks {
	dataURL := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(htmlContent))

	return chromedp.Tasks{
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible(`svg`, chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Set the viewport to match the SVG size
			err := emulation.SetDeviceMetricsOverride(int64(width), int64(height), 1, false).
				WithScreenOrientation(&emulation.ScreenOrientation{
					Type:  emulation.OrientationTypePortraitPrimary,
					Angle: 0,
				}).
				Do(ctx)
			if err != nil {
				return err
			}

			// Capture the screenshot as PNG
			*buf, err = page.CaptureScreenshot().
				WithQuality(90).
				WithClip(&page.Viewport{
					X:      0,
					Y:      0,
					Width:  width,
					Height: height,
					Scale:  1,
				}).
				Do(ctx)
			if err != nil {
				return err
			}
			return nil
		}),
	}
}
