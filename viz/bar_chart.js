//const input = {
//  Data: [
//    { Name: "apartment", Value: 7797 },
//    { Name: "road", Value: 6176 },
//    { Name: "house", Value: 4829 },
//    { Name: "parking", Value: 3056 },
//    { Name: "shed", Value: 2701 },
//    { Name: "biz", Value: 2517 },
//    { Name: "other", Value: 1077 },
//    { Name: "uni", Value: 826 },
//    { Name: "transit", Value: 675 },
//    { Name: "bar", Value: 605 },
//    { Name: "school", Value: 601 },
//    { Name: "open", Value: 555 },
//    { Name: "gov", Value: 403 },
//    { Name: "unknown", Value: 152 },
//  ],
//  Title: "Thefts by Location Type, 2014-2022";
//}
//  entryName = "Location",
//  entryValueName = "Thefts",

const baseHeight = 750,
  titleSize = "1.6em",
  baseWidth = 675,
  yLabelSize = "1.1em",
  margin = { top: 40, right: 220, bottom: 0, left: 0 },
  colors = [
    "#D32360",
    "#ED3242",
    "#E2871F",
    "#FFD515",
    "#00A168",
    "#00B1C1",
    "#108DF6",
    "#7035E6",
  ],
  dollarFormatter = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });

const width = baseWidth - margin.left - margin.right;
const height = baseHeight - margin.top - margin.bottom;

const y = d3
    .scaleBand()
    .domain(input.Data.map((d) => d.Name))
    .range([0, height])
    .padding(0.1),
  yAxis = d3.axisLeft(y).tickSize(0),
  x = d3
    .scaleLinear()
    .domain([0, d3.max(input.Data, (d) => d.Value)])
    .range([0, width]),
  xAxis = d3
    .axisTop(x)
    .ticks(width / 60, ",d")
    .tickSize(0);

const svg = d3
  .select("body")
  .append("svg")
  .attr("width", baseWidth)
  .attr("height", baseHeight);

const chart = svg
  .append("g")
  .attr("transform", "translate(" + margin.left + "," + margin.top + ")");

chart
  .append("text")
  .attr("x", (baseWidth / 2))
  .attr("y", ((-1 * margin.top) / 2) + 2)
  .attr("text-anchor", "middle")
  .attr("fill", "#0E0E14")
  .style("font-weight", "bold")
  .style("text-decoration", "Underline")
  .text(input.Title);

const bars = chart.selectAll(".bar").data(input.Data).enter();

bars
  .append("rect")
  .attr("y", (d) => y(d.Name))
  .attr("height", y.bandwidth())
  .attr("x", 0)
  .attr("width", (d) => x(d.Value))
  .attr("fill", (d, i) => colors[i % colors.length]);

// Add actual value at end of each bar
bars
  .append("text")
  .attr("y", (d) => y(d.Name) + y.bandwidth() / 2)
  .attr("x", (d) => baseWidth - 5)
  .attr("dy", ".35em")
  .attr("text-anchor", "end")
  .attr("fill", "currentColor")
  .style("font-weight", "bold")
  .text((d) => {
    if (input.IsCurrency) {
      return dollarFormatter.format(d.Value);
    } else {
      return d.Value.toLocaleString();
    }
  });

// Add y-axis labels over the bars
bars
  .append("text")
  .attr("y", (d) => y(d.Name) + y.bandwidth() / 2)
  .attr("x", 3)
  .attr("dy", ".35em")
  .attr("text-anchor", "start")
  .attr("fill", "#0E0E14")
 // .attr("fill", (d, i) =>
 //   [0, 1].includes(i % colors.length) ? "#FFD515" : "#E33266"
 // )
  .style("font-weight", "bold")
  .style("paint-order", "stroke")
  .style("stroke-width", "0.025em")
  .style("stroke", "#F0F2FA")
  .text((d) => d.Name);
