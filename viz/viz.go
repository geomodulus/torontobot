// Package viz contains code for displaying data visualizations.
package viz

import (
	"bytes"
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

	"github.com/geomodulus/torontobot/storage"
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
	Theme        string
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
<html lang="en" class="REPLACE_ME_WITH_THEME">
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
<body class="bg-map-50 dark:bg-map-900 font-mono">
    <script type="text/javascript">
    // Add your D3.js visualization code here
	  REPLACE_ME_WITH_CHART_JS
    </script>
</body>
</html>
`

// GenerateBarChartHTML generates an bare HTML file containing only styles, fonts and.
func GenerateBarChartHTML(title string, data []*DataEntry, isCurrency, darkMode bool, options ...ChartOption) (string, error) {
	js, err := GenerateBarChartJS("body", title, data, isCurrency, options...)
	if err != nil {
		return "", fmt.Errorf("generating js: %v", err)
	}
	var theme string
	if darkMode {
		theme = "dark"
	}
	themedHTML, err := strings.Replace(htmlContent, "REPLACE_ME_WITH_THEME", theme, 1), nil
	if err != nil {
		return "", fmt.Errorf("replacing theme: %v", err)
	}
	return strings.Replace(themedHTML, "REPLACE_ME_WITH_CHART_JS", js, 1), nil
}

// GenerateLineChartHTML generates an bare HTML file containing only styles, fonts and.
func GenerateLineChartHTML(title string, data []*DataEntry, isCurrency, darkMode bool, options ...ChartOption) (string, error) {
	js, err := GenerateLineChartJS("body", title, data, isCurrency, options...)
	if err != nil {
		return "", fmt.Errorf("generating js: %v", err)
	}
	var theme string
	if darkMode {
		theme = "dark"
	}
	themedHTML, err := strings.Replace(htmlContent, "REPLACE_ME_WITH_THEME", theme, 1), nil
	if err != nil {
		return "", fmt.Errorf("replacing theme: %v", err)
	}
	return strings.Replace(themedHTML, "REPLACE_ME_WITH_CHART_JS", js, 1), nil
}

type ScreenshotOptions struct {
	Width, Height, Scale float64
}

type ScreenshotOption func(*ScreenshotOptions)

func WithScale(scale float64) ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.Scale = scale
	}
}

func WithWidth(width float64) ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.Width = width
	}
}

func WithHeight(height float64) ScreenshotOption {
	return func(o *ScreenshotOptions) {
		o.Height = height
	}
}

func ScreenshotHTML(ctx context.Context, srcHTML string, options ...ScreenshotOption) ([]byte, error) {
	opts := ScreenshotOptions{
		Width:  1280,
		Height: 720,
		Scale:  1,
	}
	for _, option := range options {
		option(&opts)
	}

	ctx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 75*time.Second)
	defer cancel()

	var buf []byte
	if err := chromedp.Run(ctx, saveScreenshotPNG(srcHTML, opts.Width, opts.Height, opts.Scale, &buf)); err != nil {
		return []byte{}, fmt.Errorf("running chromedp: %v", err)
	}
	return buf, nil
}

func saveScreenshotPNG(htmlContent string, width, height, scale float64, buf *[]byte) chromedp.Tasks {
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
					Scale:  scale,
				}).
				Do(ctx)
			if err != nil {
				return err
			}
			return nil
		}),
	}
}

func GenerateAndUploadFeatureImage(ctx context.Context, id, title string, data []*DataEntry, isCurrency bool) (string, error) {
	chartHTML, err := GenerateBarChartHTML(
		title, data, isCurrency, true, //  yes to dark mode
		WithFixedWidth(800),
		WithFixedHeight(750),
	)
	if err != nil {
		return "", fmt.Errorf("generating bar chart: %v", err)
	}
	pngBytes, err := ScreenshotHTML(ctx, chartHTML, WithWidth(800), WithHeight(450), WithScale(2))
	if err != nil {
		return "", fmt.Errorf("generating PNG: %v", err)
	}
	featureImageObject := id + ".png"
	if err := storage.UploadToGCS(ctx, featureImageObject, bytes.NewReader(pngBytes)); err != nil {
		return "", fmt.Errorf("saving chart to GCS: %v", err)
	}
	return "https://dev.geomodul.us/dev-charts/" + id + ".png", nil
}

func RenderGraphJS(chartJS string) string {
	return chartJS + `

    const fc ={
      "type": "FeatureCollection",
      "features": [
        {
          "type": "Feature",
          "geometry": {
            "type": "Point",	
            "coordinates": [-79.38385, 43.65318]
          },
          "properties": {
            "name": "City Hall",
            "symbols-layer": {
              "icon-image": "city-of-toronto"
            },
            "circle-interactions-layer": {},
            "description": "<div class=\"space-y-2 mb-2\"><h1 class=\"text-blue-600 dark:text-blue-300\">City of Toronto Open Data</h1><p>Operating Budget Program Summary by Expenditure Category, 2014 - 2023</p><p><a href=\"https://open.toronto.ca/dataset/budget-operating-budget-program-summary-by-expenditure-category/\" target=\"_blank\">View on Open Data</a></p></div>",
			"popupAnchor": "bottom",
			"popupOffset": [0, -100]
          }
        }
      ]
    };
	module.displayFeatures("open-data", fc);

    module.onLoad(() =>
      module.ctx.spriteLayers.forEach((layer) => {
        module.map.setLayerZoomRange(
          layer.id,
          module.map.getMinZoom(),
          module.map.getMaxZoom()
        );
      })
    );
`
}

func RenderBody(question, schemaThoughts, analysis, sqlQuery string) string {
	return `
				<figure>
				  <div id="torontobot-chart"></div>
				  <figcaption>Data from: Operating Budget Program Summary by Expenditure Category, 2014 - 2023
				    Source: <a href="https://open.toronto.ca/dataset/budget-operating-budget-program-summary-by-expenditure-category/" target="_blank">
				    City of Toronto Open Data</a>
				  </figcaption>
				</figure>
				<p>This chart was generated using an experimental AI-powered open data query tool called 
				<a href="https://github.com/geomodulus/torontobot" target="_blank">TorontoBot</a>.</p>
				<p>Want to generate your own or help contribute to the project?
				<a href="https://discord.gg/sQzxHBq8Q2" target="_blank">Join our Discord</a>.</p>
				<ins class="geomodcau"></ins>
				<h3>How does it work?</h3>
				<p>First, the bot uses GPT-3 to analyze the question and generate a SQL query.</p>
				<p>Then, the it uses a custom SQL query engine to query a database we've filled
				with data from the City of Toronto Open Data portal.</p>
				<p>Finally, it uses a custom charting engine to generate a chart from the results.</p>
				<h3>What does the bot think?</h3>
				<h5 class="font-bold">Question</h5>
				<p>` + question + `</p>
				<h5 class="font-bold">AI thought process</h5>
				<p><em>` + schemaThoughts + `</em></p>
				<p><em>` + analysis + `</em></p>
				<h5 class="font-bold">SQL Query</h5>
				<p class="p-2 bg-map-800 text-map-200"><code>` + sqlQuery + `</code></p>`
}
