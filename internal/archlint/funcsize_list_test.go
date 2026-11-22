package archlint

// oversizedTable lists the functions over the size limits when the rule came in (17 §4
// "Function size"): file, function, then its complexity and lines at that time. Each may
// shrink but not grow, and its line goes once it is within the limits. Reason for every
// line: written before the rule; to be split. Never add a line. A plain table, not a Go
// map, so that deleting a line never realigns the others.
const oversizedTable = `
`
