//const input = {
//  Selector: "body",
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

const bars = chart.selectAll(".bar").data(input.Data).enter();

bars
  .append("rect")
  .attr("y", (d) => y(dataLabel(d)))
  .attr("height", y.bandwidth())
  .attr("x", 0)
  .attr("width", (d) => x(d.Value))
  .attr("fill", (d, i) => colors[i % colors.length]);

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
