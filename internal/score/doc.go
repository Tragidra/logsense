// Package score assigns a priority (0–100) to each cluster and records anomaly
// flags such as "burst", "novel", "rare", and "cross-service".
//
// Runner maintains a sliding window per cluster, scoring all active clusters on
// each tick. The score is a weighted sum of level severity, event frequency, burst ratio (current vs. recent average),
// rarity (fewer than 5 total events), and cross-service spread.
package score
