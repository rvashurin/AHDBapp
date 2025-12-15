package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

//go:embed web/*
var embeddedWebFS embed.FS

type server struct {
	db *sql.DB
}

type realmFaction struct {
	Realm   string `json:"realm"`
	Faction string `json:"faction"`
}

type item struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	ShortID int    `json:"shortId"`
}

type seriesPoint struct {
	ScanID int64   `json:"scanId"`
	TS     int64   `json:"ts"`
	N      int     `json:"n"`
	Min    float64 `json:"min"`
	Q1     float64 `json:"q1"`
	Q3     float64 `json:"q3"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Stddev float64 `json:"stddev"`
}

type seriesResponse struct {
	Item    item          `json:"item"`
	Realm   string        `json:"realm"`
	Faction string        `json:"faction"`
	Unit    string        `json:"unit"`
	From    int64         `json:"from"`
	To      int64         `json:"to"`
	TrimPct int           `json:"trimPct"`
	Points  []seriesPoint `json:"points"`
}

type histogramBin struct {
	Lo    int64 `json:"lo"`
	Hi    int64 `json:"hi"`
	Count int   `json:"count"`
}

type histogramResponse struct {
	ItemID  string         `json:"itemId"`
	ScanID  int64          `json:"scanId"`
	TS      int64          `json:"ts"`
	Unit    string         `json:"unit"`
	TrimPct int            `json:"trimPct"`
	N       int            `json:"n"`
	Min     int64          `json:"min"`
	Max     int64          `json:"max"`
	Bins    []histogramBin `json:"bins"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseConnectionInfo(s string) (net, addr string, _ error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "tcp", ":3306", nil
	}
	open := strings.IndexByte(s, '(')
	close := strings.LastIndexByte(s, ')')
	if open < 0 || close != len(s)-1 || close < open+2 {
		return "", "", fmt.Errorf("invalid MYSQL_CONNECTION_INFO %q (expected e.g. tcp(:3306) or unix(/path/to.sock))", s)
	}
	net = strings.TrimSpace(s[:open])
	addr = strings.TrimSpace(s[open+1 : close])
	if net == "" || addr == "" {
		return "", "", fmt.Errorf("invalid MYSQL_CONNECTION_INFO %q (empty net/addr)", s)
	}
	return net, addr, nil
}

func mysqlDSN() (string, error) {
	user := getenv("MYSQL_USER", "root")
	passwd := os.Getenv("MYSQL_PASSWORD")
	conn := getenv("MYSQL_CONNECTION_INFO", "tcp(:3306)")
	net, addr, err := parseConnectionInfo(conn)
	if err != nil {
		return "", err
	}
	cfg := mysql.NewConfig()
	cfg.User = user
	cfg.Passwd = passwd
	cfg.Net = net
	cfg.Addr = addr
	cfg.DBName = "ahdb"
	cfg.Params = map[string]string{
		"charset":   "utf8mb4",
		"parseTime": "true",
		"loc":       "UTC",
	}
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 30 * time.Second
	cfg.WriteTimeout = 30 * time.Second
	return cfg.FormatDSN(), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func (s *server) handleRealms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT realm, faction FROM scanmeta ORDER BY realm, faction`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var res []realmFaction
	for rows.Next() {
		var rf realmFaction
		if err := rows.Scan(&rf.Realm, &rf.Faction); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		res = append(res, rf)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		q = strings.TrimSpace(r.URL.Query().Get("query"))
	}
	if len(q) < 2 {
		writeJSON(w, http.StatusOK, []item{})
		return
	}
	if len(q) > 64 {
		q = q[:64]
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, shortid FROM items WHERE name LIKE ? ORDER BY name LIMIT 50`,
		"%"+q+"%",
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	res := make([]item, 0, 50)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.Name, &it.ShortID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		res = append(res, it)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *server) defaultRealmFaction(ctx context.Context) (realmFaction, error) {
	var rf realmFaction
	err := s.db.QueryRowContext(ctx, `SELECT realm, faction FROM scanmeta ORDER BY ts DESC LIMIT 1`).Scan(&rf.Realm, &rf.Faction)
	if err != nil {
		return realmFaction{}, err
	}
	return rf, nil
}

type scanAccumulator struct {
	scanID int64
	ts     int64
	prices []int64
}

func medianSorted(values []int64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return float64(values[n/2])
	}
	lo := values[n/2-1]
	hi := values[n/2]
	return float64(lo+hi) / 2
}

func (a *scanAccumulator) reset(scanID, ts int64) {
	a.scanID = scanID
	a.ts = ts
	a.prices = a.prices[:0]
}

func (a *scanAccumulator) add(price int64) {
	a.prices = append(a.prices, price)
}

func trimSorted(values []int64, trimPct int) []int64 {
	if trimPct <= 0 {
		return values
	}
	n := len(values)
	if n == 0 {
		return values
	}
	trim := int(math.Floor(float64(n) * (float64(trimPct) / 100.0)))
	maxTrim := (n - 1) / 2
	if trim > maxTrim {
		trim = maxTrim
	}
	return values[trim : n-trim]
}

func (a *scanAccumulator) point(trimPct int) seriesPoint {
	if len(a.prices) == 0 {
		return seriesPoint{}
	}
	prices := trimSorted(a.prices, trimPct)
	n := len(prices)
	if n == 0 {
		return seriesPoint{}
	}

	var mean float64
	var m2 float64
	for i := range prices {
		x := float64(prices[i])
		delta := x - mean
		mean += delta / float64(i+1)
		delta2 := x - mean
		m2 += delta * delta2
	}

	minV := float64(prices[0])
	maxV := float64(prices[n-1])
	median := medianSorted(prices)
	q1 := median
	q3 := median
	if n > 1 {
		var lower []int64
		var upper []int64
		if n%2 == 0 {
			lower = prices[:n/2]
			upper = prices[n/2:]
		} else {
			lower = prices[:n/2]
			upper = prices[n/2+1:]
		}
		if len(lower) > 0 {
			q1 = medianSorted(lower)
		}
		if len(upper) > 0 {
			q3 = medianSorted(upper)
		}
	}
	var variance float64
	if n > 0 {
		variance = m2 / float64(n)
	}
	return seriesPoint{
		ScanID: a.scanID,
		TS:     a.ts,
		N:      n,
		Min:    minV,
		Q1:     q1,
		Q3:     q3,
		Max:    maxV,
		Mean:   mean,
		Median: median,
		Stddev: math.Sqrt(variance),
	}
}

func parseIntParam(r *http.Request, key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return v, nil
}

func parseMaxPointsParam(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("maxPoints"))
	if raw == "" {
		return 400, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid maxPoints")
	}
	if v < 10 {
		return 10, nil
	}
	if v > 5000 {
		return 5000, nil
	}
	return v, nil
}

func parseTrimPctParam(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("trimPct"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("trim"))
	}
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid trimPct")
	}
	if v < 0 || v > 50 {
		return 0, errors.New("trimPct must be between 0 and 50")
	}
	if v%5 != 0 {
		return 0, errors.New("trimPct must be a multiple of 5")
	}
	return v, nil
}

func parseBinsParam(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("bins"))
	if raw == "" {
		return 24, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid bins")
	}
	if v < 5 {
		return 5, nil
	}
	if v > 120 {
		return 120, nil
	}
	return v, nil
}

func parseScanIDParam(r *http.Request) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("scanId"))
	if raw == "" {
		return 0, errors.New("missing scanId")
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v <= 0 {
		return 0, errors.New("invalid scanId")
	}
	return v, nil
}

func makeHistogram(sortedPrices []int64, bins int) (min int64, max int64, res []histogramBin) {
	n := len(sortedPrices)
	if n == 0 {
		return 0, 0, nil
	}
	min = sortedPrices[0]
	max = sortedPrices[n-1]
	if min == max {
		return min, max, []histogramBin{{Lo: min, Hi: max, Count: n}}
	}
	if bins <= 0 {
		bins = 24
	}

	rng := max - min
	width := rng / int64(bins)
	if rng%int64(bins) != 0 {
		width++
	}
	if width < 1 {
		width = 1
	}

	res = make([]histogramBin, bins)
	for i := 0; i < bins; i++ {
		lo := min + int64(i)*width
		hi := lo + width
		res[i] = histogramBin{Lo: lo, Hi: hi}
	}
	for _, p := range sortedPrices {
		idx := int((p - min) / width)
		if idx < 0 {
			idx = 0
		}
		if idx >= bins {
			idx = bins - 1
		}
		res[idx].Count++
	}
	return min, max, res
}

func (s *server) handleSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	itemID := strings.TrimSpace(r.URL.Query().Get("itemId"))
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "missing itemId")
		return
	}

	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	if unit == "" {
		unit = "per_item"
	}
	var priceExpr string
	switch unit {
	case "per_item":
		priceExpr = "CAST(ROUND(a.buyout / a.itemCount) AS SIGNED)"
	case "per_stack":
		priceExpr = "a.buyout"
	default:
		writeError(w, http.StatusBadRequest, "invalid unit (expected per_item or per_stack)")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	realm := strings.TrimSpace(r.URL.Query().Get("realm"))
	faction := strings.TrimSpace(r.URL.Query().Get("faction"))
	if realm == "" || faction == "" {
		rf, err := s.defaultRealmFaction(ctx)
		if err != nil {
			writeError(w, http.StatusBadRequest, "missing realm/faction and no default available")
			return
		}
		if realm == "" {
			realm = rf.Realm
		}
		if faction == "" {
			faction = rf.Faction
		}
	}

	now := time.Now().Unix()
	to, err := parseIntParam(r, "to", now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	from, err := parseIntParam(r, "from", -1)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if from < 0 {
		days, err := parseIntParam(r, "days", 7)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if days <= 0 {
			from = 0
		} else {
			from = to - days*86400
		}
	}
	if from > to {
		writeError(w, http.StatusBadRequest, "from must be <= to")
		return
	}

	maxPoints, err := parseMaxPointsParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	trimPct, err := parseTrimPctParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var it item
	err = s.db.QueryRowContext(ctx, `SELECT id, name, shortid FROM items WHERE id = ? LIMIT 1`, itemID).Scan(&it.ID, &it.Name, &it.ShortID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	query := fmt.Sprintf(`
SELECT a.scanId, UNIX_TIMESTAMP(s.ts) AS ts, %s AS price
FROM auctions a
JOIN scanmeta s ON s.id = a.scanId
WHERE a.itemId = ?
  AND a.buyout > 0
  AND a.itemCount > 0
  AND s.realm = ?
  AND s.faction = ?
  AND s.ts BETWEEN FROM_UNIXTIME(?) AND FROM_UNIXTIME(?)
ORDER BY a.scanId, price`, priceExpr)

	rows, err := s.db.QueryContext(ctx, query, itemID, realm, faction, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var points []seriesPoint
	acc := scanAccumulator{prices: make([]int64, 0, 256)}
	var curScanID int64 = -1
	var curTS int64
	for rows.Next() {
		var scanID int64
		var ts int64
		var price int64
		if err := rows.Scan(&scanID, &ts, &price); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if curScanID == -1 {
			curScanID = scanID
			curTS = ts
			acc.reset(scanID, ts)
		}
		if scanID != curScanID {
			points = append(points, acc.point(trimPct))
			curScanID = scanID
			curTS = ts
			acc.reset(scanID, ts)
		}
		if ts != curTS {
			acc.ts = ts
			curTS = ts
		}
		acc.add(price)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(acc.prices) > 0 {
		points = append(points, acc.point(trimPct))
	}

	sort.Slice(points, func(i, j int) bool { return points[i].TS < points[j].TS })
	if len(points) > maxPoints {
		points = points[len(points)-maxPoints:]
	}

	writeJSON(w, http.StatusOK, seriesResponse{
		Item:    it,
		Realm:   realm,
		Faction: faction,
		Unit:    unit,
		From:    from,
		To:      to,
		TrimPct: trimPct,
		Points:  points,
	})
}

func (s *server) handleHistogram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	itemID := strings.TrimSpace(r.URL.Query().Get("itemId"))
	if itemID == "" {
		writeError(w, http.StatusBadRequest, "missing itemId")
		return
	}
	scanID, err := parseScanIDParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	unit := strings.TrimSpace(r.URL.Query().Get("unit"))
	if unit == "" {
		unit = "per_item"
	}
	var priceExpr string
	switch unit {
	case "per_item":
		priceExpr = "CAST(ROUND(a.buyout / a.itemCount) AS SIGNED)"
	case "per_stack":
		priceExpr = "a.buyout"
	default:
		writeError(w, http.StatusBadRequest, "invalid unit (expected per_item or per_stack)")
		return
	}

	trimPct, err := parseTrimPctParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	bins, err := parseBinsParam(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	query := fmt.Sprintf(`
SELECT UNIX_TIMESTAMP(s.ts) AS ts, %s AS price
FROM auctions a
JOIN scanmeta s ON s.id = a.scanId
WHERE a.scanId = ?
  AND a.itemId = ?
  AND a.buyout > 0
  AND a.itemCount > 0
ORDER BY price`, priceExpr)

	rows, err := s.db.QueryContext(ctx, query, scanID, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var ts int64
	prices := make([]int64, 0, 256)
	for rows.Next() {
		var rowTS int64
		var price int64
		if err := rows.Scan(&rowTS, &price); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ts = rowTS
		prices = append(prices, price)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	prices = trimSorted(prices, trimPct)
	minV, maxV, hbins := makeHistogram(prices, bins)
	writeJSON(w, http.StatusOK, histogramResponse{
		ItemID:  itemID,
		ScanID:  scanID,
		TS:      ts,
		Unit:    unit,
		TrimPct: trimPct,
		N:       len(prices),
		Min:     minV,
		Max:     maxV,
		Bins:    hbins,
	})
}

func main() {
	var addr string
	flag.StringVar(&addr, "addr", "127.0.0.1:8080", "listen address")
	flag.Parse()

	dsn, err := mysqlDSN()
	if err != nil {
		log.Fatalf("DB config error: %v", err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("DB open error: %v", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("DB ping error: %v", err)
	}

	webFS, err := fs.Sub(embeddedWebFS, "web")
	if err != nil {
		log.Fatalf("web assets error: %v", err)
	}

	s := &server{db: db}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/realms", s.handleRealms)
	mux.HandleFunc("/api/items", s.handleItems)
	mux.HandleFunc("/api/series", s.handleSeries)
	mux.HandleFunc("/api/histogram", s.handleHistogram)
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("Listening on http://%s", addr)
	log.Fatal(httpServer.ListenAndServe())
}
