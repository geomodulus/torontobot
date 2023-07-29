//const input = {
//  Selector: "body",
//  Keys: ["Value1", "Value2"],
//  Data: [
//    { Name: "apartment", Value1: 7797, Value2: 3450 },
//    { Name: "road", Value: 6176, Value2: 1788 },
//    { Name: "house", Value: 4829, Value2: 1850 },
//    { Name: "parking", Value: 3056, Value2: 1550 },
//    { Name: "shed", Value: 2701, Value2: 1450 },
//    { Name: "biz", Value: 2517, Value2: 13000 },
//    { Name: "other", Value: 1077, Value2: 504 },
//    { Name: "uni", Value: 826, Value2: 488 },
//    { Name: "transit", Value: 675, Value2: 375 },
//    { Name: "bar", Value: 605, Value2: 228 },
//    { Name: "school", Value: 601, Value2: 391 },
//    { Name: "open", Value: 555, Value2: 281 },
//    { Name: "gov", Value: 403, Value2: 111 },
//    { Name: "unknown", Value: 152, Value2: 59 },
//  ],
//  Title: "Thefts by Location Type, 2014-2022";
//}
//  entryName = "Location",
//  entryValueName = "Thefts",

// baseWidth is set dynamically.
// baseHeight is set dynamically.

const margin = { top: 50, right: 210, bottom: 0, left: 0 },
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
  dollarFormatter = new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
  });

const width = baseWidth - margin.left - margin.right;
const height = baseHeight - margin.top - margin.bottom;

const y = d3
  .scaleBand()
  .domain(input.Data.map((d) => dataLabel(d)))
  .range([0, height])
  .padding(0.1);

const x = d3
  .scaleLinear()
  .domain([0, d3.max(input.Data, (d) => d.Value1 + d.Value2)])
  .range([0, width]);

const yAxis = d3.axisLeft(y).tickSize(0);
const xAxis = d3
  .axisTop(x)
  .ticks(width / 60, ",d")
  .tickSize(0);

const color = d3
  .scaleOrdinal()
  .range(["#6b486b", "#a05d56", "#d0743c", "#ff8c00"]);
const stack = d3.stack().keys(["Value1", "Value2"])(input.Data);

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
  .attr("class", "text-sm md:text-lg")
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

// Create bars
const bars = chart
  .selectAll("g")
  .data(stack)
  .enter()
  .append("g")
  .attr("fill", (d) => color(d.key))
  .selectAll("rect")
  .data((d) => d)
  .enter()
  .append("rect")
  .attr("y", (d) => y(dataLabel(d.data)))
  .attr("height", y.bandwidth())
  .attr("x", (d) => x(d[0]))
  .attr("width", (d) => x(d[1]) - x(d[0]));

// Add actual value at end of each bar
bars
  .append("text")
  .attr("y", (d) => y(dataLabel(d)) + y.bandwidth() / 2)
  .attr("x", (d) => baseWidth - 5)
  .attr("dy", ".35em")
  .attr("class", "text-xs md:text-base")
  .attr("text-anchor", "end")
  .attr("fill", "currentColor")
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
  .attr("y", (d) => y(dataLabel(d)) + y.bandwidth() / 2)
  .attr("x", 3)
  .attr("dy", ".35em")
  .attr("class", "text-xs md:text-base")
  .attr("text-anchor", "start")
  .attr("fill", "currentColor")
  .style("font-weight", "bold")
  .each(function (d) {
    if (dataLabel(d).length > 30) {
      const [firstPart, secondPart] = splitAtWordBoundary(dataLabel(d), 28);

      d3.select(this).append("tspan").attr("dy", "-0.2em").text(firstPart);

      if (secondPart) {
        d3.select(this)
          .append("tspan")
          .attr("x", 3)
          .attr("dy", "1.3em")
          .text(secondPart);
      }
    } else {
      d3.select(this).text(dataLabel(d));
    }
  });

// Add legend
const legend = svg
  .selectAll(".legend")
  .data(color.domain().slice().reverse())
  .enter()
  .append("g")
  .attr("class", "legend")
  .attr("transform", (d, i) => "translate(0," + i * 20 + ")");

legend
  .append("rect")
  .attr("x", width - 18)
  .attr("width", 18)
  .attr("height", 18)
  .attr("fill", color);

legend
  .append("text")
  .attr("x", width - 24)
  .attr("y", 9)
  .attr("dy", ".35em")
  .style("text-anchor", "end")
  .text((d) => (d === "Value1" ? "Type 1" : "Type 2")); // replace "Type 1" and "Type 2" with your actual types

function splitAtWordBoundary(str, limit) {
  if (str.length <= limit) {
    return [str, ""];
  }

  const regex = new RegExp(`^.{0,${limit}}\\b`);
  const firstPart = str.match(regex)[0];
  const secondPart = str.slice(firstPart.length);

  return [firstPart, secondPart];
}

function dataLabel(d) {
  if (d.Date) return d.Date;
  return d.Name;
}
