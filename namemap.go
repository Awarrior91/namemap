package namemap

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/fractalqb/xsx"
	"github.com/fractalqb/xsx/gem"
	"github.com/fractalqb/xsx/table"
)

type terms = []string
type termMap = map[string]terms

// NameMap is the basic data structure to map a name from one input
// domain to another. Use Load or MustLoad to read a NameMap from a
// definition file.
type NameMap struct {
	domIdxs map[string]int
	stdDom  int
	trmMaps []termMap
}

// DomainIdx returns the index of a domain given the domains
// name. Using the index for name mapping avoids string look ups and
// should be used in performance critical applications. If no domain
// with the given domain name exists, -1 is returned.
func (nm *NameMap) DomainIdx(domain string) int {
	idx, ok := nm.domIdxs[domain]
	if ok {
		return idx
	} else {
		return -1
	}
}

func (nm *NameMap) DomainName(idx int) string {
	for nm, i := range nm.domIdxs {
		if i == idx {
			return nm
		}
	}
	return ""
}

// IgnDom ignores the domain of a mapped name. This can be used for all .Map
// and .MapNm methods if the domain is irrelevant.
func IgnDom(mapped string, ignore int) string { return mapped }

// Map maps 'term' from 'fromDomain' to the corresponding name in the
// first matching 'toDomains' element. If no matching 'toDomains'
// element is found the 'term' itself is returned as 'mapped' and
// 'domain' is -1. Otherwise 'domain' contains the domain index of the
// matching target domain.
func (nm *NameMap) Map(fromDomain int, term string, toDomains ...int) (mapped string, domain int) {
	tmap := nm.trmMaps[fromDomain]
	trms, ok := tmap[term]
	if ok {
		for _, to := range toDomains {
			if to < 0 || to >= len(trms) {
				continue
			}
			if mapped = trms[to]; len(mapped) > 0 {
				return mapped, to
			}
		}
	}
	return term, -1
}

// MapNm determines all domain indices for 'fromNm' and 'toNames' and
// calls NameMap.Map().
func (nm *NameMap) MapNm(fromNm string, term string, toNames ...string) (string, int) {
	fromIdx := nm.DomainIdx(fromNm)
	if fromIdx < 0 {
		return term, -1
	}
	toIdxs := make([]int, len(toNames))
	for i, name := range toNames {
		toIdxs[i] = nm.DomainIdx(name)
	}
	return nm.Map(fromIdx, term, toIdxs...)
}

// MustLoad loads the NameMap definition from file 'filename' and
// panics if an error occurs.
func MustLoad(filename string) *NameMap {
	frd, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer frd.Close()
	res := &NameMap{}
	err = res.Load(frd)
	if err != nil {
		panic(err)
	}
	return res
}

// Load load the definition of the NameMap from 'rd'. Definitions are
// texts formatted as an XSX table (see
// https://godoc.org/github.com/fractalqb/xsx/table).
func (nm *NameMap) Load(rd io.Reader) (err error) {
	xrd := xsx.NewPullParser(bufio.NewReader(rd))
	tDef, err := nm.loadDoms(xrd)
	if err != nil {
		return err
	}
	nm.trmMaps = make([]termMap, len(nm.domIdxs))
	for i := 0; i < len(nm.trmMaps); i++ {
		nm.trmMaps[i] = make(termMap)
	}
	for row, err := tDef.NextRow(xrd, nil); row != nil; row, err = tDef.NextRow(xrd, nil) {
		if err != nil {
			return err
		}
		err := nm.loadTerm(row)
		if err != nil {
			return err
		}
	}
	return nil
}

func (nm *NameMap) loadDoms(xrd *xsx.PullParser) (table.Definition, error) {
	tDef, err := table.ReadDef(xrd)
	if err != nil {
		return nil, err
	}
	nm.domIdxs = make(map[string]int)
	nm.stdDom = -1
	for i, col := range tDef {
		if _, ok := nm.domIdxs[col.Name]; ok {
			return nil, fmt.Errorf("pull namemap: duplicate domain '%s'", col.Name)
		}
		nm.domIdxs[col.Name] = i
		if col.Meta {
			if nm.stdDom >= 0 {
				return nil, errors.New("pull namemap: ambiguous standard domain")
			}
			nm.stdDom = i
		}
	}
	if len(nm.domIdxs) == 0 {
		return nil, errors.New("empty domain definition")
	}
	return tDef, nil
}

func (nm *NameMap) loadTerm(term []gem.Expr) error {
	tStrs := make([]string, len(term))
	for i := 0; i < len(nm.domIdxs); i++ {
		switch atom := term[i].(type) {
		case *gem.Atom:
			if atom.Meta() {
				tStrs[i] = ""
			} else {
				tStrs[i] = atom.Str
				nm.trmMaps[i][atom.Str] = tStrs
			}
		default:
			return fmt.Errorf("name-map load: item %d is not an atom", i)
		}
	}
	return nil
}

func (nm *NameMap) Save(wr io.Writer) (err error) {
	xpr := xsx.Pretty(wr)
	if err = xpr.Begin('[', false); err != nil {
		return err
	}
	for dom, idx := range nm.domIdxs {
		if err = xpr.Atom(dom, idx == nm.stdDom, xsx.Qcond); err != nil {
			return err
		}
	}
	if err = xpr.End(); err != nil {
		return err
	}
	stdTMap := nm.trmMaps[nm.stdDom]
	for _, terms := range stdTMap {
		if err = xpr.Begin('(', false); err != nil {
			return err
		}
		for _, term := range terms {
			xpr.Atom(term, false, xsx.Qcond)
		}
		if err = xpr.End(); err != nil {
			return err
		}
	}
	return nil
}

func (nm *NameMap) ForEach(domain int, apply func(value string)) {
	tmap := nm.trmMaps[domain]
	for t := range tmap {
		apply(t)
	}
}

type UnknownDomain struct {
	MapHint    string
	DomainHint string
}

func (err *UnknownDomain) Error() string {
	res := fmt.Sprintf("unknown domain '%s' in map '%s'", err.DomainHint, err.MapHint)
	return res
}

type From struct {
	nmap *NameMap
	fIdx int
}

func (nm NameMap) From(fromDomain string, fallback bool) From {
	res := From{&nm, nm.DomainIdx(fromDomain)}
	if res.fIdx < 0 && fallback {
		res.fIdx = nm.stdDom
	}
	return res
}

func (nm NameMap) FromStd() From {
	res := From{&nm, nm.stdDom}
	return res
}

func (nm *From) Check(mapHint string, domainHint string) error {
	if nm.fIdx < 0 {
		return &UnknownDomain{mapHint, domainHint}
	}
	return nil
}

func (nm From) Verify(mapHint string, domainHint string) From {
	err := nm.Check(mapHint, domainHint)
	if err != nil {
		panic(err)
	}
	return nm
}

func (fnm *From) Map(term string, toDomains ...int) (mapped string, domain int) {
	mapped, domain = fnm.nmap.Map(fnm.fIdx, term, toDomains...)
	return mapped, domain
}

func (fnm *From) MapNm(term string, toNames ...string) (string, int) {
	toIdxs := make([]int, len(toNames))
	for i, name := range toNames {
		toIdxs[i] = fnm.Base().DomainIdx(name)
	}
	return fnm.Map(term, toIdxs...)
}

func (fnm *From) Base() *NameMap { return fnm.nmap }

type To struct {
	nmap  *NameMap
	tIdxs []int
}

func (nm NameMap) To(appendStd bool, toDomains ...string) To {
	haveStd := false
	res := To{nmap: &nm}
	for _, tDom := range toDomains {
		idx := nm.DomainIdx(tDom)
		if idx >= 0 {
			res.tIdxs = append(res.tIdxs, idx)
			haveStd = haveStd || (idx == nm.stdDom)
		}
	}
	if appendStd && !haveStd {
		res.tIdxs = append(res.tIdxs, nm.stdDom)
	}
	return res
}

func (nm *To) Check(mapHint string, domainHint string) error {
	if len(nm.tIdxs) == 0 {
		return &UnknownDomain{mapHint, domainHint}
	}
	return nil
}

func (nm To) Verify(mapHint string, domainHint string) To {
	err := nm.Check(mapHint, domainHint)
	if err != nil {
		panic(err)
	}
	return nm
}

func (tnm *To) Map(fromDomain int, term string) (mapped string, domain int) {
	for dIdx, tDom := range tnm.tIdxs {
		mapped, domain = tnm.nmap.Map(fromDomain, term, tDom)
		if domain >= 0 {
			return mapped, dIdx
		}
	}
	return term, -1
}

func (tnm *To) MapNm(fromName string, term string) (string, int) {
	toIdx := tnm.Base().DomainIdx(fromName)
	if toIdx < 0 {
		return term, -1
	}
	return tnm.Map(toIdx, term)
}

func (tnm *To) Base() *NameMap { return tnm.nmap }

func (fnm From) To(appendStd bool, toDomains ...string) FromTo {
	toMap := fnm.Base().To(appendStd, toDomains...)
	res := FromTo{&toMap, fnm.fIdx}
	return res
}

func (tnm To) From(fromDomain string, fallback bool) FromTo {
	res := FromTo{&tnm, tnm.Base().DomainIdx(fromDomain)}
	if res.fIdx < 0 && fallback {
		res.fIdx = res.Base().stdDom
	}
	return res
}

func (tnm To) FromStd() FromTo {
	res := FromTo{&tnm, tnm.Base().stdDom}
	return res
}

type FromTo struct {
	tomap *To
	fIdx  int
}

func (nm *FromTo) Check(mapHint string, domainHint string) error {
	if nm.fIdx < 0 {
		return &UnknownDomain{mapHint, domainHint}
	} else if err := nm.tomap.Check(mapHint, domainHint); err != nil {
		return err
	}
	return nil
}

func (nm FromTo) Verify(mapHint string, domainHint string) FromTo {
	err := nm.Check(mapHint, domainHint)
	if err != nil {
		panic(err)
	}
	return nm
}

func (nm *FromTo) Map(term string) (mapped string, domain int) {
	mapped, domain = nm.tomap.Map(nm.fIdx, term)
	return mapped, domain
}

func (nm *FromTo) Base() *NameMap { return nm.tomap.Base() }
