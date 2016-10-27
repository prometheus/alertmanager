// Setup

// Modify the diameter to expand/contract space between nodes.
var diameter = 1240;

var tree = d3.layout.tree()
    .size([360, diameter / 2 - 120])
    .separation(function(a, b) { return (a.parent == b.parent ? 1 : 2) / a.depth; });

var diagonal = d3.svg.diagonal.radial()
    .projection(function(d) { return [d.y, d.x / 180 * Math.PI]; });

var svg;

var tooltip = d3.select("body")
    .append("div")
    .style("position", "absolute")
    .style("background-color", "white")
    .style("border", "1px solid #ddd")
    .style("font", "9px monospace")
    .style("padding", "4px 2px")
    .style("z-index", "10")
    .style("visibility", "hidden");

function parseSearch(searchString) {
  var labels = searchString.replace(/{|}|\"|\s/g, "").split(",");
  var o = {};
  labels.forEach(function(label) {
    var l = label.split("=");
    o[l[0]] = l[1];
  });
  return o;
}

function resetSVG() {
  d3.select(".js-svg svg").remove()
  svg = d3.select(".js-svg").append("svg")
    .attr("width", diameter)
    .attr("height", diameter - 150)
    .append("g")
    .attr("transform", "translate(" + diameter / 2 + "," + (diameter / 2 - 200) + ")");
}

// Handler for reading config.yml
d3.json("api/v1/status", function(error, data) {
  var parsedConfig = jsyaml.load(data.data.config);

  // Create a new SVG for each time a config is loaded.
  resetSVG();
  loadConfig(parsedConfig);
});

// Click handler for input labelSet
d3.select(".js-find-match").on("click", function() {
  var searchValue = document.querySelector("input").value
  var labelSet = parseSearch(searchValue);
  var matches = match(root, labelSet)
  var nodes = tree.nodes(root);
  var matchedIds = matches.map(function(n) { return n.id; });
  nodes.forEach(function(n) {
    if (matchedIds.indexOf(n.id) > -1) {
      n.matched = true;
    } else {
      n.matched = false;
    }
  });
  update(root);
});

// Match does a depth-first left-to-right search through the route tree
// and returns the matching routing nodes.
function match(root, labelSet) {
  // See if the node is a match. If it is, recurse through the children.
  if (!matchLabels(root.matchers, labelSet)) {
    return [];
  }

  var all = []

  if (root.children) {
    for (var j = 0; j < root.children.length; j++) {
      var child = root.children[j];
      var matches = match(child, labelSet);

      all = all.concat(matches);

      if (matches.length && !child.continue) {
        break;
      }
    }
  }

  // If no child nodes were matches, the current node itself is a match.
  if (all.length === 0) {
    all.push(root);
  }

  return all
}

// Compare set of matchers to labelSet
function matchLabels(matchers, labelSet) {
  for (var j = 0; j < matchers.length; j++) {
    if (!matchLabel(matchers[j], labelSet)) {
      return false;
    }
  }
  return true;
}

// Compare single matcher to labelSet
function matchLabel(matcher, labelSet) {
  var v = labelSet[matcher.name];

  if (matcher.isRegex) {
    return matcher.value.test(v)
  }

  return matcher.value === v;
}

// Load the parsed config and create the tree
function loadConfig(config) {
  root = config.route;

  root.parent = null;
  massage(root)

  update(root);
}

// Translate AlertManager names to expected d3 tree names, convert AlertManager
// Match and MatchRE objects to js objects.
function massage(root) {
  if (!root) return;

  root.children = root.routes

  var matchers = []
  if (root.match) {
    for (var key in root.match) {
      var o = {};
      o.isRegex = false;
      o.value = root.match[key];
      o.name = key;
      matchers.push(o);
    }
  }

  if (root.match_re) {
    for (var key in root.match_re) {
      var o = {};
      o.isRegex = true;
      o.value = new RegExp(root.match_re[key]);
      o.name = key;
      matchers.push(o);
    }
  }

  root.matchers = matchers;

  if (!root.children) return;

  root.children.forEach(function(child) {
    child.parent = root;
    massage(child)
  });
}

// Update the tree based on root.
function update(root) {
  var i = 0;
  var nodes = tree.nodes(root);
  var links = tree.links(nodes);

  var matchedNodes = nodes.filter(function(n) { return n.matched })
  var highlight = [];
  if (matchedNodes.length) {
    highlight = matchedNodes
    matchedNodes.forEach(function(n) {
      var mn = n
      while (mn.parent) {
        highlight.push(mn.parent);
        mn = mn.parent;
      }
    });
  }

  var link = svg.selectAll(".link").data(links);

  link.enter().append("path")
    .attr("class", "link")
    .attr("d", diagonal);

  if (highlight.length) {
    link.style("stroke", function(d) {
      if (highlight.indexOf(d.source) > -1 && highlight.indexOf(d.target) > -1) {
        return "steelblue"
      }
      return "#ccc"
    });
  }

  var node = svg.selectAll(".node")
    .data(nodes, function(d) { return d.id || (d.id = ++i); });

  var nodeEnter = node.enter().append("g")
    .attr("class", "node")
    .attr("transform", function(d) {
      return "rotate(" + (d.x - 90) + ")translate(" + d.y + ")";
    })

  nodeEnter.append("circle")
      .attr("r", 4.5);

  nodeEnter.append("text")
      .attr("dy", ".31em")
      .attr("text-anchor", function(d) { return d.x < 180 ? "start" : "end"; })
      .attr("transform", function(d) { return d.x < 180 ? "translate(8)" : "rotate(180)translate(-8)"; })
      .text(function(d) { return d.receiver; });

  node.select(".node circle").style("fill", function(d) {
    return d.matched ? "steelblue" : "#fff";
  })
  .on("mouseover", function(d) {
    d3.select(this).style("fill", "steelblue");

    // Show all matchers for node and ancestors
    matchers = aggregateMatchers(d);
    text = formatMatcherText(matchers);
    text.forEach(function(t) {
      tooltip.append("div").text(t);
    });
    if (text.length) {
      return tooltip.style("visibility", "visible");
    }
  })
  .on("mousemove", function() {
    return tooltip.style("top", (d3.event.pageY-10)+"px").style("left",(d3.event.pageX+10)+"px");
  })
  .on("mouseout", function(d) {
    d3.select(this).style("fill", d.matched ? "steelblue" : "#fff");
    tooltip.text("");
    return tooltip.style("visibility", "hidden");
  });
}

function formatMatcherText(matchersArray) {
  return matchersArray.map(function(m) {
    return m.name + ": " + m.value;
  });
}

function aggregateMatchers(node) {
  var n = node
  matchers = [];
  while (n.parent) {
    matchers = matchers.concat(n.matchers);
    n = n.parent;
  }
  return matchers
}
