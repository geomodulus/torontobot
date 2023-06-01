//const input = {
//  Selector: "body",
//  Data: [
 // {
 //   Name: "Parking Lots",
 //   Value: 3476,
 // },
 // {
 //   Name: "House",
 //   Value: 1584,
 // },
 // {
 //   Name: "Apartment",
 //   Value: 1324,
 // },
 // {
 //   Name: "Streets/Roads",
 //   Value: 1154,
 // },
 // {
 //   Name: "Other",
 //   Value: 888 + 26 + 19 + 137,
 // },
 // {
 //   Name: "Commercial",
 //   Value: 575,
 // },
//  ],
//  Title: "Library Spending By Year, 2014-2022";
//},
//  baseWidth = 675;
//  entryName = "Location",
//  entryValueName = "Thefts",
// baseWidth is setDynamically.

const margin = { top: 35, right: 0, bottom: 0, left: 20 };

const width = baseWidth - margin.left - margin.right;
const height = baseHeight - margin.top - margin.bottom;

const radius = Math.min(width, height) / 2.5;

let outerRadius = radius - 25,
  innerRadius = radius - 90;

const svg = d3
  .select(input.Selector)
  .append("svg")
  .attr("width", width + margin.left + margin.right)
  .attr("height", height + margin.top + margin.bottom);

const chart = svg
  .append("g")
  .attr(
    "transform",
    "translate(" +
      (width / 2 + margin.left) +
      "," +
      (height / 2 + margin.top) +
      ")"
  );

const title = svg
  .append("text")
  .attr("x", baseWidth / 2)
  .attr("y", margin.top + 4)
  .attr("text-anchor", "middle")
  .attr("fill", "currentColor")
  .attr("class", "text-base md:text-lg")
  .style("text-decoration", "Underline")

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

const customColors = [
  "#F6505C",
  "#E2871F",
  "#FFD515",
  "#00A168",
  "#00B1C1",
  "#108DF6",
  "#7035E6",
  "#D32360",
];

const color = d3
  .scaleOrdinal()
  .domain(input.Data.map((d) => d.Name))
  .range(customColors);

const pie = d3.pie().value(function (d) {
  return d.Value;
});

const path = d3.arc().outerRadius(outerRadius).innerRadius(innerRadius);

const label = d3
  .arc()
  .outerRadius(radius + 10)
  .innerRadius(radius + 10);

const arc = chart
  .selectAll(".arc")
  .data(pie(input.Data))
  .enter()
  .append("g")
  .attr("class", "arc");

arc
  .append("path")
  .attr("d", path)
  .attr("fill", function (d) {
    return color(d.data.Value);
  });

  const totalCount = d3.sum(input.Data, (d) => d.Value);

  const sliceLabel = arc
    .append("text")
    .attr("transform", function (d) {
      return "translate(" + label.centroid(d) + ")";
    })
    //.attr("dy", "0.35em")
    .attr("fill", "currentColor")
    .style("alignment-baseline", "middle")
    .style("text-anchor", "middle");
  sliceLabel
    .append("tspan")
    .style("paint-order", "stroke")
    .attr("class", "text-base md:text-lg stroke-gray-100 dark:stroke-gray-800")
    .style("stroke-width", "0.1em")
    .attr("dy", 0)
    .attr("x", 0)
    .text(function (d) {
      return d.data.Name;
    });
  sliceLabel
    .append("tspan")
    .attr("x", 0)
    .attr("dy", "1.2em")
    .style("paint-order", "stroke")
    .attr("class", "text-xs md:text-sm stroke-gray-100 dark:stroke-gray-800")
    .style("stroke-width", "0.1em")
    .text(function (d) {
      const percentage = ((d.data.Value / totalCount) * 100).toFixed(1);
      return `${percentage}%`;
    });


// Add total
//const total = chart
//  .append("text")
//  .attr("x", 0)
//  .attr("y", 0)
//  .attr("text-anchor", "middle")
//  .style("paint-order", "stroke")
//  .style("stroke-width", "0.1em")
//  .style(
//    "stroke",
//    module.isDarkMode() ? module.colors.gray[800] : module.colors.gray[100]
//  )
//  .attr("fill", "currentColor");
//total
//  .append("tspan")
//  .attr("x", 0)
//  .attr("dy", "-0.2em")
//  .style("font-size", totalFontSize)
//  .text(totalCount.toLocaleString());
//total
//  .append("tspan")
//  .attr("x", 0)
//  .attr("dy", "1.4em")
//  .style("font-size", labelSize)
//  .text("thefts reported");
//total
//  .append("tspan")
//  .attr("x", 0)
//  .attr("dy", "1.4em")
//  .style("font-size", labelSize)
//  .text("in 2022");

function splitAtWordBoundary(str, limit) {
  if (str.length <= limit) {
    return [str, ""];
  }

  const regex = new RegExp(`^.{0,${limit}}\\b`);
  const firstPart = str.match(regex)[0];
  const secondPart = str.slice(firstPart.length);

  return [firstPart, secondPart];
}


