//const input = {
//  Selector: "body",
//  Data: [
//    {Date: 2014, "Value": 184220135},
//    {Date: 2015, "Value": 188708307.09},
//    {Date: 2016, "Value": 193461896},
//    {Date: 2017, "Value": 199047201.96},
//    {Date: 2018, "Value": 201606806.09},
//    {Date: 2019, "Value": 206880105.49},
//    {Date: 2020, "Value": 215819410.34},
//    {Date: 2021, "Value": 221576306.59},
//    {Date: 2022, "Value": 228305383.13},
//    {Date: 2023, "Value": 234610256.67}
//  ],
//  IsCurrency: true,
//  Title: "Library Spending By Year, 2014-2022";
//},
//  baseWidth = 675;
//  entryName = "Location",
//  entryValueName = "Thefts",
// baseWidth is setDynamically.

const baseHeight = 400,
  titleSize = "1.6em",
  yLabelSize = "1.1em",
  margin = { top: 35, right: 0, bottom: 0, left: 0 },
  colors = [
    "#D32360",
    "#ED3242",
    "#E2871F",
    "#FDA400",
    "#00A168",
    "#00B1C1",
    "#108DF6",
    "#7035E6",
  ],
  dollarFormatter = new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
  });

const width = baseWidth - margin.left - margin.right;
const height = baseHeight - margin.top - margin.bottom;

const svg = d3
  .select(input.Selector)
  .append("svg")
  .attr("width", baseWidth)
  .attr("height", baseHeight);

const chart = svg
  .append("g")
  .attr("transform", "translate(" + margin.left + "," + margin.top + ")");

const title = chart
  .append("text")
  .attr("x", baseWidth / 2)
  .attr("y", (-1 * margin.top) / 2 + 2)
  .attr("text-anchor", "middle")
  .attr("fill", "currentColor")
  .style("font-weight", "bold")
  .style("text-decoration", "Underline");
if (input.Title.length > 42) {
  const parts = splitAtWordBoundary(input.Title, 40);
  title
    .append("tspan")
    .attr("x", baseWidth / 2)
    .attr("dy", "-0.3em")
    .text(function (d) {
      return parts[0];
    });
  title
    .append("tspan")
    .attr("x", baseWidth / 2)
    .attr("dy", "1.2em")
    .text(function (d) {
      return parts[1];
    });
} else {
  title.text(input.Title);
}

const x = d3
  .scaleTime()
  .domain(
    d3.extent(input.Data, function (d) {
      return d.Date;
    })
  )
  .range([0, width]);
chart
  .append("g")
  .attr("transform", "translate(0," + height + ")")
  .call(d3.axisBottom(x));

const y = d3
  .scaleLinear()
  .domain([
    0,
    d3.max(input.Data, function (d) {
      return +d.Value;
    }),
  ])
  .range([height, 0]);
chart.append("g").call(d3.axisLeft(y));

chart
  .append("path")
  .datum(input.Data)
  .attr("fill", "none")
  .attr("stroke", colors[0])
  .attr("stroke-width", 1.5)
  .attr(
    "d",
    d3
      .line()
      .x(function (d) {
        return x(d.Date);
      })
      .y(function (d) {
        return y(d.Value);
      })
  );

function splitAtWordBoundary(str, limit) {
  if (str.length <= limit) {
    return [str, ""];
  }

  const regex = new RegExp(`^.{0,${limit}}\\b`);
  const firstPart = str.match(regex)[0];
  const secondPart = str.slice(firstPart.length);

  return [firstPart, secondPart];
}
