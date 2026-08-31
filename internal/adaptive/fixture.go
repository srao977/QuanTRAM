package adaptive

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"quantram/internal/domain"
)

type UnitRunRow struct {
	Index                int
	SourceRowIndex       int
	SourceTimestamp      string
	EntityID             string
	Open                 float64
	High                 float64
	Low                  float64
	Close                float64
	Volume               uint64
	PhysicalRow          int
	Status               string
	PathDirection        domain.PathDirection
	PositionDecision     string
	H                    int
	QG                   float64
	QS                   float64
	QR                   float64
	C                    float64
	Strength             float64
	Coherence            float64
	Persistence          float64
	Uncertainty          float64
	Reversal             float64
	TerminalDisplacement float64
	PositionAfter        domain.EmitterPosition
}

func LoadUnitRun001(path string) ([]UnitRunRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.ReuseRecord = true
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	idx := indexHeader(header)
	var rows []UnitRunRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		row, err := parseUnitRunRow(rec, idx)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func FileSHA256(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func (row UnitRunRow) Bar() (domain.Bar, error) {
	return BarFromOHLCV(row.EntityID, row.SourceTimestamp, row.Open, row.High, row.Low, row.Close, row.Volume)
}

func indexHeader(header []string) map[string]int {
	out := make(map[string]int, len(header))
	for i, name := range header {
		out[name] = i
	}
	return out
}

func parseUnitRunRow(rec []string, idx map[string]int) (UnitRunRow, error) {
	get := func(name string) string {
		i, ok := idx[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return rec[i]
	}
	vol, err := ParseVolume(get("volume"))
	if err != nil {
		return UnitRunRow{}, err
	}
	h, err := strconv.Atoi(strings.TrimSuffix(get("H"), ".0"))
	if err != nil {
		hf, herr := strconv.ParseFloat(get("H"), 64)
		if herr != nil {
			return UnitRunRow{}, fmt.Errorf("H: %w", err)
		}
		h = int(hf)
	}
	row := UnitRunRow{
		Index:                atoi(get("pipeline_observation_number")),
		SourceRowIndex:       atoi(get("source_row_index")),
		SourceTimestamp:      get("source_timestamp"),
		EntityID:             get("entity_id"),
		Open:                 atof(get("open")),
		High:                 atof(get("high")),
		Low:                  atof(get("low")),
		Close:                atof(get("close")),
		Volume:               vol,
		PhysicalRow:          atoi(get("physical_row")),
		Status:               get("status"),
		PathDirection:        domain.PathDirection(get("path_direction")),
		PositionDecision:     get("position_decision"),
		H:                    h,
		QG:                   atof(get("Q_G")),
		QS:                   atof(get("Q_S")),
		QR:                   atof(get("Q_R")),
		C:                    atof(get("C")),
		Strength:             atof(get("strength")),
		Coherence:            atof(get("coherence")),
		Persistence:          atof(get("persistence")),
		Uncertainty:          atof(get("uncertainty")),
		Reversal:             atof(get("reversal_propensity")),
		TerminalDisplacement: atof(get("terminal_displacement")),
		PositionAfter:        domain.EmitterPosition(get("position_state_after")),
	}
	return row, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
