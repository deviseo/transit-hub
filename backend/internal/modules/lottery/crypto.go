package lottery

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newID(prefix string) (string, error) {
	value, err := randomHex(16)
	if err != nil {
		return "", err
	}
	return prefix + "_" + value, nil
}

func seedCommitment(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func finalSeed(secret string, campaignID string, snapshotHash string) []byte {
	sum := sha256.Sum256([]byte(secret + campaignID + snapshotHash))
	return sum[:]
}

func snapshotHash(entries []Entry) (string, error) {
	rows := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, map[string]string{"entryId": entry.ID, "sub2apiUserId": entry.Sub2apiUserID, "receiptHash": entry.ReceiptHash})
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// snapshotHashForVersion preserves the original private v1 contract for
// already-published campaigns. New v2 campaigns hash only fields exposed by
// the public entry register, so any viewer can independently rebuild it.
func snapshotHashForVersion(entries []Entry, version string) (string, error) {
	if version == "" || version == AlgorithmVersionV1 {
		return snapshotHash(entries)
	}
	if version != AlgorithmVersionV2 {
		return "", errors.New("unsupported lottery algorithm version")
	}
	rows := make([]map[string]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, map[string]string{"entryId": entry.ID, "maskedEmail": entry.MaskedEmail, "receiptHash": entry.ReceiptHash})
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func deterministicShuffle(entries []Entry, seed []byte) ([]Entry, error) {
	shuffled := append([]Entry(nil), entries...)
	for i := len(shuffled) - 1; i > 0; i-- {
		j, err := unbiasedIndex(seed, i+1, len(shuffled)-i)
		if err != nil {
			return nil, err
		}
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	return shuffled, nil
}

func unbiasedIndex(seed []byte, n int, counter int) (int, error) {
	if n <= 0 {
		return 0, errors.New("invalid shuffle bound")
	}
	limit := ^uint64(0) - (^uint64(0) % uint64(n))
	for attempt := uint64(0); ; attempt++ {
		mac := hmac.New(sha256.New, seed)
		var buf [16]byte
		binary.BigEndian.PutUint64(buf[:8], uint64(counter))
		binary.BigEndian.PutUint64(buf[8:], attempt)
		_, _ = mac.Write(buf[:])
		sum := mac.Sum(nil)
		value := binary.BigEndian.Uint64(sum[:8])
		if value < limit {
			return int(value % uint64(n)), nil
		}
	}
}

func expandedPrizeSlots(prizes []Prize) []Prize {
	sorted := append([]Prize(nil), prizes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].SortOrder == sorted[j].SortOrder {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].SortOrder < sorted[j].SortOrder
	})
	slots := make([]Prize, 0)
	for _, prize := range sorted {
		for i := 0; i < prize.Quantity; i++ {
			slots = append(slots, prize)
		}
	}
	return slots
}

func parseShanghaiTime(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		utc := parsed.UTC()
		return &utc, nil
	}
	loc, err := time.LoadLocation(DefaultLotteryTimezone)
	if err != nil {
		return nil, err
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05", time.DateOnly} {
		if parsed, err := time.ParseInLocation(layout, trimmed, loc); err == nil {
			utc := parsed.UTC()
			return &utc, nil
		}
	}
	return nil, requestError(ErrorValidation)
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return formatTime(*t)
}
