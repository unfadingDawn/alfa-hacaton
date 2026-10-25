// Package semantic implements local, non-generative semantic ranking with
// TF-IDF and latent semantic analysis (LSA). It does not call an AI service.
package semantic

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"math/rand/v2"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gonum.org/v1/gonum/mat"
)

type Config struct {
	MaxDocuments int
	MaxFeatures  int
	Dimensions   int
	MinDocuments int
}

type Document struct {
	ID      string
	Title   string
	Summary string
}

type Representation struct {
	TFIDF  map[int]float64
	Latent []float64
}

type Similarity struct {
	TFIDF    float64
	LSA      float64
	Combined float64
}

type Model struct {
	vocabulary map[string]int
	idf        []float64
	projection *mat.Dense // features x latent dimensions
	version    string
	documents  int
}

var wordRE = regexp.MustCompile(`[\p{L}\p{N}]+`)

var stopWords = map[string]bool{
	"это": true, "как": true, "что": true, "для": true, "или": true, "при": true, "под": true, "над": true,
	"его": true, "ее": true, "она": true, "они": true, "оно": true, "мы": true, "вы": true, "их": true, "из": true,
	"на": true, "по": true, "до": true, "от": true, "за": true, "со": true, "но": true, "не": true, "уже": true,
	"будет": true, "были": true, "был": true, "была": true, "также": true, "the": true, "and": true, "for": true,
	"with": true, "from": true, "into": true, "are": true, "was": true, "were": true,
}

func normalize(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "ё", "е"))
	return strings.Join(wordRE.FindAllString(s, -1), " ")
}

// terms combines word features, adjacent word pairs and character n-grams.
// Character features make Russian inflections less brittle without requiring
// a language-specific stemmer.
func terms(text string) map[string]int {
	words := strings.Fields(normalize(text))
	filtered := make([]string, 0, len(words))
	out := map[string]int{}
	for _, word := range words {
		runes := []rune(word)
		if len(runes) < 3 || stopWords[word] {
			continue
		}
		filtered = append(filtered, word)
		out["w:"+word]++
		if len(runes) >= 5 {
			for n := 3; n <= 5; n++ {
				if len(runes) < n {
					break
				}
				for i := 0; i+n <= len(runes); i++ {
					out["c:"+string(runes[i:i+n])]++
				}
			}
		}
	}
	for i := 1; i < len(filtered); i++ {
		out["b:"+filtered[i-1]+"_"+filtered[i]]++
	}
	return out
}

func documentTerms(d Document) map[string]int {
	out := terms(d.Summary)
	// Title terms get triple weight relative to summary terms.
	for term, n := range terms(d.Title) {
		out[term] += 3 * n
	}
	return out
}

type candidate struct {
	term  string
	score float64
}

func Fit(documents []Document, cfg Config) (*Model, error) {
	if cfg.MinDocuments <= 0 {
		cfg.MinDocuments = 10
	}
	if len(documents) < cfg.MinDocuments {
		return nil, errors.New("недостаточно документов для семантической модели")
	}
	if cfg.MaxFeatures <= 0 {
		cfg.MaxFeatures = 3000
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 64
	}

	docTerms := make([]map[string]int, len(documents))
	df, total := map[string]int{}, map[string]int{}
	versionHash := sha256.New()
	versionHash.Write([]byte("tfidf-lsa-v1:" + strconv.Itoa(cfg.MaxFeatures) + ":" + strconv.Itoa(cfg.Dimensions) + "\x00"))
	for i, d := range documents {
		docTerms[i] = documentTerms(d)
		seen := map[string]bool{}
		for term, n := range docTerms[i] {
			total[term] += n
			if !seen[term] {
				df[term]++
				seen[term] = true
			}
		}
		versionHash.Write([]byte(d.ID + "\x00" + d.Title + "\x00" + d.Summary + "\x00"))
	}

	minDF := 2
	if len(documents) < 20 {
		minDF = 1
	}
	maxDF := int(math.Ceil(float64(len(documents)) * 0.9))
	candidates := make([]candidate, 0, len(df))
	for term, freq := range df {
		if freq < minDF || freq > maxDF {
			continue
		}
		idf := math.Log((float64(len(documents))+1)/(float64(freq)+1)) + 1
		candidates = append(candidates, candidate{term: term, score: math.Log1p(float64(total[term])) * idf})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].term < candidates[j].term
	})
	if len(candidates) > cfg.MaxFeatures {
		candidates = candidates[:cfg.MaxFeatures]
	}
	if len(candidates) < 2 {
		return nil, errors.New("недостаточно признаков для семантической модели")
	}
	// Stable indices make the model reproducible when candidate scores tie.
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].term < candidates[j].term })
	m := &Model{vocabulary: make(map[string]int, len(candidates)), idf: make([]float64, len(candidates)), documents: len(documents)}
	for i, c := range candidates {
		m.vocabulary[c.term] = i
		m.idf[i] = math.Log((float64(len(documents))+1)/(float64(df[c.term])+1)) + 1
	}

	x := make([]map[int]float64, len(documents))
	for i, counts := range docTerms {
		x[i] = m.vectorize(counts)
	}
	m.projection = randomizedProjection(x, len(m.idf), cfg.Dimensions)
	sum := versionHash.Sum(nil)
	m.version = "lsa:" + hex.EncodeToString(sum[:6])
	return m, nil
}

func (m *Model) Version() string { return m.version }
func (m *Model) Documents() int  { return m.documents }
func (m *Model) Dimensions() int {
	if m.projection == nil {
		return 0
	}
	_, n := m.projection.Dims()
	return n
}

func (m *Model) vectorize(counts map[string]int) map[int]float64 {
	v := make(map[int]float64, len(counts))
	norm := 0.0
	for term, count := range counts {
		i, ok := m.vocabulary[term]
		if !ok || count <= 0 {
			continue
		}
		value := (1 + math.Log(float64(count))) * m.idf[i]
		v[i] = value
		norm += value * value
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i, value := range v {
			v[i] = value / norm
		}
	}
	return v
}

func (m *Model) Represent(text string) Representation {
	tfidf := m.vectorize(terms(text))
	return Representation{TFIDF: tfidf, Latent: project(tfidf, m.projection)}
}

func (m *Model) RepresentDocument(title, summary string) Representation {
	tfidf := m.vectorize(documentTerms(Document{Title: title, Summary: summary}))
	return Representation{TFIDF: tfidf, Latent: project(tfidf, m.projection)}
}

func Compare(a, b Representation) Similarity {
	tf := sparseCosine(a.TFIDF, b.TFIDF)
	lsa := denseCosine(a.Latent, b.Latent)
	combined := tf
	if len(a.Latent) > 0 && len(b.Latent) > 0 {
		combined = 0.35*tf + 0.65*lsa
	}
	return Similarity{TFIDF: clamp01(tf), LSA: clamp01(lsa), Combined: clamp01(combined)}
}

func (m *Model) Similarity(profile Representation, title, summary string) Similarity {
	return Compare(profile, m.RepresentDocument(title, summary))
}

func project(v map[int]float64, projection *mat.Dense) []float64 {
	if len(v) == 0 || projection == nil {
		return nil
	}
	features, dims := projection.Dims()
	out := make([]float64, dims)
	for i, value := range v {
		if i >= features {
			continue
		}
		for j := 0; j < dims; j++ {
			out[j] += value * projection.At(i, j)
		}
	}
	normalizeDense(out)
	return out
}

func sparseCosine(a, b map[int]float64) float64 {
	if len(a) > len(b) {
		a, b = b, a
	}
	dot := 0.0
	for i, v := range a {
		dot += v * b[i]
	}
	return dot // vectorize already applies L2 normalization
}

func denseCosine(a, b []float64) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	dot := 0.0
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot
}

func normalizeDense(v []float64) {
	norm := 0.0
	for _, x := range v {
		norm += x * x
	}
	if norm == 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] /= norm
	}
}

func clamp01(v float64) float64 {
	if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// randomizedProjection returns the leading right singular vectors of a sparse
// document-term matrix. Only the small projected matrix is factorized by SVD.
func randomizedProjection(x []map[int]float64, features, dimensions int) *mat.Dense {
	n := len(x)
	k := min(dimensions, n-1, features)
	if k < 1 {
		return nil
	}
	l := min(k+8, n, features)
	rng := rand.New(rand.NewPCG(0xA1FA, 0x2026))
	omega := make([][]float64, features)
	for i := range omega {
		omega[i] = make([]float64, l)
		for j := range omega[i] {
			omega[i][j] = rng.NormFloat64()
		}
	}
	y := make([][]float64, n)
	for d, row := range x {
		y[d] = make([]float64, l)
		for feature, value := range row {
			for j := 0; j < l; j++ {
				y[d][j] += value * omega[feature][j]
			}
		}
	}
	q := orthonormalColumns(y, l)
	if len(q) == 0 || len(q[0]) == 0 {
		return nil
	}
	actual := len(q[0])
	b := mat.NewDense(actual, features, nil)
	for d, row := range x {
		for feature, value := range row {
			for j := 0; j < actual; j++ {
				b.Set(j, feature, b.At(j, feature)+q[d][j]*value)
			}
		}
	}
	var svd mat.SVD
	if !svd.Factorize(b, mat.SVDThin) {
		return nil
	}
	values := svd.Values(nil)
	usable := min(k, len(values))
	for usable > 0 && values[usable-1] < 1e-9 {
		usable--
	}
	if usable == 0 {
		return nil
	}
	var allV mat.Dense
	svd.VTo(&allV)
	projection := mat.NewDense(features, usable, nil)
	for i := 0; i < features; i++ {
		for j := 0; j < usable; j++ {
			projection.Set(i, j, allV.At(i, j))
		}
	}
	return projection
}

func orthonormalColumns(y [][]float64, columns int) [][]float64 {
	if len(y) == 0 {
		return nil
	}
	q := make([][]float64, len(y))
	for i := range q {
		q[i] = make([]float64, 0, columns)
	}
	for col := 0; col < columns; col++ {
		v := make([]float64, len(y))
		for i := range y {
			v[i] = y[i][col]
		}
		// Re-orthogonalize once for numerical stability.
		for pass := 0; pass < 2; pass++ {
			for prev := 0; prev < len(q[0]); prev++ {
				dot := 0.0
				for i := range v {
					dot += v[i] * q[i][prev]
				}
				for i := range v {
					v[i] -= dot * q[i][prev]
				}
			}
		}
		norm := 0.0
		for _, value := range v {
			norm += value * value
		}
		if norm < 1e-18 {
			continue
		}
		norm = math.Sqrt(norm)
		for i := range v {
			q[i] = append(q[i], v[i]/norm)
		}
	}
	return q
}
