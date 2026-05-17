/*
 * @licstart This JavaScript code is licensed under the AGPL 3.0 license.
 * See https://git.sr.ht/~sircmpwn/git.sr.ht/tree/master/LICENSE for details.
 * @licend
 */

/**
 * Matches URL hash selecting one or more lines:
 * - #L10           - single line
 * - #L10,20        - multiple lines
 * - #L10-15        - span of lines
 * - #L10-15,20-25  - multiple spans
 * - #L10,15-20,30  - combination of above
 */
const hashPattern = /^#L(\d+(-\d+)?)(,\d+(-\d+)?)*$/;

const isValidHash = hash => hash.match(hashPattern);

const getLine = no => document.getElementById(`L${no}`);

const getLineCount = () => document.querySelectorAll('.lines > a').length;

const lineNumber = line => Number(line.id.substring(1));

function* range(start, end) {
  if (end < start) {
    [start, end] = [end, start];
  }

  for (let n = start; n <= end; n += 1) {
    yield n;
  }
}

/**
 * Given a string representation of a span returns the numbers contained in it.
 * Numbers greater than max are ignored.
 */
const parseSpan = (span, max) => {
  const [sStart, sEnd] = span.includes("-") ? span.split("-") : [span, span];
  const [start, end] = [sStart, sEnd].map(Number).sort((a, b) => a - b);

  if (start > max) {
    return [];
  } else if (end > max) {
    return range(start, max);
  } else {
    return range(start, end);
  }
}

/**
 * Returns a set of line numbers matching the hash.
 */
const lineNumbersFromHash = hash => {
  const lineCount = getLineCount();
  const lineNos = new Set();

  if (isValidHash(hash)) {
    const spans = location.hash.substring(2).split(",");
    for (let span of spans) {
      for (let no of parseSpan(span, lineCount)) {
        lineNos.add(no);
      }
    }
  }

  return lineNos;
}

/**
 * Given a set of line numbers, groups them into spans.
 * Yields tuples of [startNo, endNo].
 */
const spansFromLineNumbers = lineNos => {
  if (lineNos.size === 0) {
    return [];
  }

  const sorted = Array.from(lineNos).sort((a, b) => a - b);
  const spans = [];
  let current, prev;
  let start = sorted[0];

  for (current of sorted) {
    if (prev && current !== prev + 1) {
      spans.push([start, prev]);
      start = current;
    }
    prev = current;
  }
  spans.push([start, current]);

  return spans;
}

/**
 * Returns a hash matching the given set of line numbers.
 */
const hashFromLineNumbers = lineNos => {
  if (lineNos.size === 0) {
    return "";
  }

  const spans = spansFromLineNumbers(lineNos);
  const parts = [];

  for ([start, end] of spans) {
    if (start == end) {
      parts.push(start);
    } else {
      parts.push([start, end].join("-"));
    }
  }

  return "#L" + parts.join(",");
}

const selectLine = lineNo => {
  const line = getLine(lineNo);
  if (line) {
    line.classList.add("selected");
  }
}

const selectLines = lineNos => {
  for (lineNo of lineNos) {
    selectLine(lineNo);
  }
}

const unselectLine = lineNo => {
  const line = getLine(lineNo);
  if (line) {
    line.classList.remove("selected");
  }
}

const unselectAll = () => {
  const selected = document.querySelectorAll(".lines .selected");
  for (let line of selected) {
    line.classList.remove("selected");
  }
}

const handlePlainClick = (selected, lineNo) => {
  selected.clear();
  selected.add(lineNo);
  unselectAll();
  selectLine(lineNo);
}

const handleCtrlClick = (selected, lineNo) => {
  if (selected.has(lineNo)) {
    selected.delete(lineNo);
    unselectLine(lineNo);
  } else {
    selected.add(lineNo);
    selectLine(lineNo);
  }
}

const handleShiftClick = (selected, lineNo, lastNo) => {
  if (lastNo) {
    for (no of range(lastNo, lineNo)) {
      selected.add(no);
      selectLine(no);
    }
  }
}

/**
 * Scroll the selected lines into view.
 */
const scrollToSelected = (selected) => {
  if (selected.size > 0) {
    const firstNo = Math.min(...selected);
    const scrollNo = Math.max(firstNo - 5, 1);  // add top padding
    const line = getLine(scrollNo);
    if (line) {
      line.scrollIntoView();
    }
  }
}

/**
 * Returns true if two sets contain the same elements.
 */
const setsEqual = (a, b) => {
  if (a.size != b.size) {
    return false;
  }
  for (n of a) {
    if (!b.has(n)) {
      return false;
    }
  }
  return true;
}

/**
 * A set of currently selected line numbers.
 */
let selected = lineNumbersFromHash(location.hash);

/**
 * The number of the last line to be clicked. Used to select spans of lines.
 * If a single line is selected initially, set to that line.
 */
let lastNo = selected.size == 1 ? Array.from(selected)[0] : null;

/**
 * Overrides default click handler for line numbers.
 */
const handleLineClicked = event => {
  event.preventDefault();

  const lineNo = lineNumber(event.target);
  if (event.ctrlKey) {
    handleCtrlClick(selected, lineNo);
  } else if (event.shiftKey) {
    handleShiftClick(selected, lineNo, lastNo);
  } else {
    handlePlainClick(selected, lineNo);
  }

  lastNo = lineNo;

  const hash = hashFromLineNumbers(selected);
  if (hash) {
    window.location.hash = hash;
  } else {
    // Hacky way to clear the hash (https://stackoverflow.com/a/15323220)
    history.pushState('', document.title, window.location.pathname);
  }
}

// Catch when the hash is changed from the outside and update the selection
// e.g. when the user edits the hash in the URL
window.onhashchange = () => {
  let newSelected = lineNumbersFromHash(location.hash);
  if (!setsEqual(selected, newSelected)) {
    selected = newSelected;
    unselectAll();
    selectLines(selected);
  }
}

document.querySelectorAll('.lines a').forEach(
  line => line.addEventListener("click", handleLineClicked)
);

// Initially select lines matching hash and scroll them into view
selectLines(selected);
scrollToSelected(selected);
