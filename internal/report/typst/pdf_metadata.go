package typst

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var pdfIndirectRefPattern = regexp.MustCompile(`(\d+)\s+(\d+)\s+R`)

// PDFMetadata describes the extra PDF metadata velocity.report wants to stamp
// onto Typst-generated reports.
type PDFMetadata struct {
	Creator  string
	Keywords []string
}

func (m PDFMetadata) empty() bool {
	return m.Creator == "" && len(m.Keywords) == 0
}

type pdfRef struct {
	number     int
	generation int
}

type pdfTrailer struct {
	size int
	root pdfRef
	info *pdfRef
	id   string
}

func applyPDFMetadata(pdf []byte, meta PDFMetadata) ([]byte, error) {
	if meta.empty() {
		return pdf, nil
	}

	startXRef, err := parseStartXRef(pdf)
	if err != nil {
		return nil, err
	}
	trailerDict, err := parseLastTrailerDict(pdf)
	if err != nil {
		return nil, err
	}
	trailer, err := parsePDFTrailer(trailerDict)
	if err != nil {
		return nil, err
	}
	xrefOffsets, err := parseXRefTable(pdf, startXRef)
	if err != nil {
		return nil, err
	}

	updatedObjects := map[int][]byte{}
	infoRef := trailer.info
	if infoRef == nil {
		newInfo := pdfRef{number: trailer.size, generation: 0}
		emptyInfo := []byte("<<>>")
		updatedObjects[newInfo.number] = buildInfoObject(newInfo, emptyInfo, meta)
		infoRef = &newInfo
		trailer.size++
	} else {
		infoOffset, ok := xrefOffsets[infoRef.number]
		if !ok {
			return nil, fmt.Errorf("pdf info object %d missing from xref", infoRef.number)
		}
		infoObj, err := readPDFObject(pdf, infoOffset)
		if err != nil {
			return nil, err
		}
		updatedInfo, err := updateInfoObject(infoObj, *infoRef, meta)
		if err != nil {
			return nil, err
		}
		updatedObjects[infoRef.number] = updatedInfo
	}

	rootOffset, ok := xrefOffsets[trailer.root.number]
	if !ok {
		return nil, fmt.Errorf("pdf catalog object %d missing from xref", trailer.root.number)
	}
	rootObj, err := readPDFObject(pdf, rootOffset)
	if err != nil {
		return nil, err
	}
	rootDict, err := extractPDFDict(rootObj)
	if err != nil {
		return nil, err
	}
	metadataRef, ok := parseIndirectRef(rootDict, "Metadata")
	if ok {
		metadataOffset, found := xrefOffsets[metadataRef.number]
		if !found {
			return nil, fmt.Errorf("pdf metadata object %d missing from xref", metadataRef.number)
		}
		metadataObj, err := readPDFObject(pdf, metadataOffset)
		if err != nil {
			return nil, err
		}
		updatedMetadata, err := updateMetadataObject(metadataObj, metadataRef, meta)
		if err != nil {
			return nil, err
		}
		updatedObjects[metadataRef.number] = updatedMetadata
	}

	return appendIncrementalUpdate(pdf, trailer, startXRef, infoRef, updatedObjects), nil
}

func updateInfoObject(obj []byte, ref pdfRef, meta PDFMetadata) ([]byte, error) {
	dict, err := extractPDFDict(obj)
	if err != nil {
		return nil, err
	}
	if meta.Creator != "" {
		dict = replaceOrAddPDFString(dict, "Creator", meta.Creator)
	}
	if len(meta.Keywords) > 0 {
		dict = replaceOrAddPDFString(dict, "Keywords", strings.Join(meta.Keywords, ", "))
	}
	return buildIndirectObject(ref, dict), nil
}

func updateMetadataObject(obj []byte, ref pdfRef, meta PDFMetadata) ([]byte, error) {
	dict, stream, err := extractPDFStreamObject(obj)
	if err != nil {
		return nil, err
	}
	if meta.Creator != "" {
		stream, err = replaceOrAddXMLTag(stream, "xmp:CreatorTool", meta.Creator)
		if err != nil {
			return nil, err
		}
	}
	if len(meta.Keywords) > 0 {
		stream, err = replaceOrAddXMLTag(stream, "pdf:Keywords", strings.Join(meta.Keywords, ", "))
		if err != nil {
			return nil, err
		}
	}
	dict = replaceOrAddPDFInt(dict, "Length", len(stream))
	return buildStreamObject(ref, dict, stream), nil
}

func appendIncrementalUpdate(pdf []byte, trailer pdfTrailer, prevXRef int, infoRef *pdfRef, objects map[int][]byte) []byte {
	var out bytes.Buffer
	out.Write(pdf)

	numbers := make([]int, 0, len(objects))
	for number := range objects {
		numbers = append(numbers, number)
	}
	sort.Ints(numbers)

	offsets := map[int]int{}
	for _, number := range numbers {
		offsets[number] = out.Len()
		out.Write(objects[number])
	}

	xrefStart := out.Len()
	out.WriteString("xref\n")
	for _, number := range numbers {
		fmt.Fprintf(&out, "%d 1\n%010d 00000 n \n", number, offsets[number])
	}
	out.WriteString("trailer\n<<")
	fmt.Fprintf(&out, " /Size %d", trailer.size)
	fmt.Fprintf(&out, " /Root %d %d R", trailer.root.number, trailer.root.generation)
	if infoRef != nil {
		fmt.Fprintf(&out, " /Info %d %d R", infoRef.number, infoRef.generation)
	}
	fmt.Fprintf(&out, " /Prev %d", prevXRef)
	if trailer.id != "" {
		fmt.Fprintf(&out, " /ID %s", trailer.id)
	}
	out.WriteString(" >>\n")
	fmt.Fprintf(&out, "startxref\n%d\n%%%%EOF\n", xrefStart)
	return out.Bytes()
}

func buildInfoObject(ref pdfRef, dict []byte, meta PDFMetadata) []byte {
	if meta.Creator != "" {
		dict = replaceOrAddPDFString(dict, "Creator", meta.Creator)
	}
	if len(meta.Keywords) > 0 {
		dict = replaceOrAddPDFString(dict, "Keywords", strings.Join(meta.Keywords, ", "))
	}
	return buildIndirectObject(ref, dict)
}

func buildIndirectObject(ref pdfRef, dict []byte) []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "%d %d obj\n", ref.number, ref.generation)
	out.Write(dict)
	out.WriteString("\nendobj\n")
	return out.Bytes()
}

func buildStreamObject(ref pdfRef, dict, stream []byte) []byte {
	var out bytes.Buffer
	fmt.Fprintf(&out, "%d %d obj\n", ref.number, ref.generation)
	out.Write(dict)
	out.WriteString("\nstream\n")
	out.Write(stream)
	out.WriteString("\nendstream\nendobj\n")
	return out.Bytes()
}

func parseStartXRef(pdf []byte) (int, error) {
	idx := bytes.LastIndex(pdf, []byte("startxref"))
	if idx < 0 {
		return 0, fmt.Errorf("pdf startxref not found")
	}
	fields := bytes.Fields(pdf[idx+len("startxref"):])
	if len(fields) == 0 {
		return 0, fmt.Errorf("pdf startxref value missing")
	}
	start, err := strconv.Atoi(string(fields[0]))
	if err != nil {
		return 0, fmt.Errorf("parse startxref: %w", err)
	}
	return start, nil
}

func parseLastTrailerDict(pdf []byte) ([]byte, error) {
	startXRefIdx := bytes.LastIndex(pdf, []byte("startxref"))
	if startXRefIdx < 0 {
		return nil, fmt.Errorf("pdf startxref marker not found")
	}
	trailerIdx := bytes.LastIndex(pdf[:startXRefIdx], []byte("trailer"))
	if trailerIdx < 0 {
		return nil, fmt.Errorf("pdf trailer not found")
	}
	dictStart := bytes.Index(pdf[trailerIdx:], []byte("<<"))
	if dictStart < 0 {
		return nil, fmt.Errorf("pdf trailer dictionary start not found")
	}
	dictStart += trailerIdx
	dictEnd := bytes.Index(pdf[dictStart:], []byte(">>"))
	if dictEnd < 0 {
		return nil, fmt.Errorf("pdf trailer dictionary end not found")
	}
	dictEnd += dictStart + 2
	return pdf[dictStart:dictEnd], nil
}

func parsePDFTrailer(dict []byte) (pdfTrailer, error) {
	size, ok := parsePDFInt(dict, "Size")
	if !ok {
		return pdfTrailer{}, fmt.Errorf("pdf trailer missing Size")
	}
	root, ok := parseIndirectRef(dict, "Root")
	if !ok {
		return pdfTrailer{}, fmt.Errorf("pdf trailer missing Root")
	}
	info, hasInfo := parseIndirectRef(dict, "Info")
	id, _ := parsePDFRawValue(dict, "ID", `\[[^\]]+\]`)

	trailer := pdfTrailer{size: size, root: root, id: id}
	if hasInfo {
		trailer.info = &info
	}
	return trailer, nil
}

func parseXRefTable(pdf []byte, start int) (map[int]int, error) {
	if start < 0 || start >= len(pdf) {
		return nil, fmt.Errorf("xref offset %d out of range", start)
	}
	if !bytes.HasPrefix(pdf[start:], []byte("xref")) {
		return nil, fmt.Errorf("unsupported pdf xref stream")
	}

	i := start + len("xref")
	offsets := map[int]int{}
	for {
		i = skipPDFWhitespace(pdf, i)
		if i >= len(pdf) {
			return nil, fmt.Errorf("unterminated xref table")
		}
		if bytes.HasPrefix(pdf[i:], []byte("trailer")) {
			return offsets, nil
		}

		line, next, err := readPDFLine(pdf, i)
		if err != nil {
			return nil, err
		}
		fields := strings.Fields(string(line))
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed xref subsection header %q", line)
		}
		startObject, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("parse xref object number: %w", err)
		}
		count, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("parse xref count: %w", err)
		}
		i = next

		for index := 0; index < count; index++ {
			line, next, err = readPDFLine(pdf, i)
			if err != nil {
				return nil, err
			}
			fields = strings.Fields(string(line))
			if len(fields) < 3 {
				return nil, fmt.Errorf("malformed xref entry %q", line)
			}
			if fields[2] == "n" {
				offset, err := strconv.Atoi(fields[0])
				if err != nil {
					return nil, fmt.Errorf("parse xref offset: %w", err)
				}
				offsets[startObject+index] = offset
			}
			i = next
		}
	}
}

func readPDFObject(pdf []byte, offset int) ([]byte, error) {
	if offset < 0 || offset >= len(pdf) {
		return nil, fmt.Errorf("pdf object offset %d out of range", offset)
	}
	end := bytes.Index(pdf[offset:], []byte("endobj"))
	if end < 0 {
		return nil, fmt.Errorf("pdf object at %d missing endobj", offset)
	}
	end += offset + len("endobj")
	return pdf[offset:end], nil
}

func extractPDFDict(obj []byte) ([]byte, error) {
	start := bytes.Index(obj, []byte("<<"))
	if start < 0 {
		return nil, fmt.Errorf("pdf object dictionary start not found")
	}
	end := bytes.Index(obj[start:], []byte(">>"))
	if end < 0 {
		return nil, fmt.Errorf("pdf object dictionary end not found")
	}
	end += start + 2
	return obj[start:end], nil
}

func extractPDFStreamObject(obj []byte) ([]byte, []byte, error) {
	dictStart := bytes.Index(obj, []byte("<<"))
	if dictStart < 0 {
		return nil, nil, fmt.Errorf("pdf stream dictionary start not found")
	}
	dictEnd := bytes.Index(obj[dictStart:], []byte(">>"))
	if dictEnd < 0 {
		return nil, nil, fmt.Errorf("pdf stream dictionary end not found")
	}
	dictEnd += dictStart + 2
	streamIdx := bytes.Index(obj[dictEnd:], []byte("stream"))
	if streamIdx < 0 {
		return nil, nil, fmt.Errorf("pdf stream marker not found")
	}
	streamIdx += dictEnd + len("stream")
	if streamIdx < len(obj) && obj[streamIdx] == '\r' {
		streamIdx++
	}
	if streamIdx < len(obj) && obj[streamIdx] == '\n' {
		streamIdx++
	}
	endStreamIdx := bytes.Index(obj[streamIdx:], []byte("endstream"))
	if endStreamIdx < 0 {
		return nil, nil, fmt.Errorf("pdf endstream marker not found")
	}
	endStreamIdx += streamIdx
	stream := bytes.TrimRight(obj[streamIdx:endStreamIdx], "\r\n")
	return obj[dictStart:dictEnd], stream, nil
}

func parseIndirectRef(dict []byte, key string) (pdfRef, bool) {
	value, ok := parsePDFRawValue(dict, key, `(\d+)\s+(\d+)\s+R`)
	if !ok {
		return pdfRef{}, false
	}
	match := pdfIndirectRefPattern.FindStringSubmatch(value)
	number, _ := strconv.Atoi(match[1])
	generation, _ := strconv.Atoi(match[2])
	return pdfRef{number: number, generation: generation}, true
}

func parsePDFInt(dict []byte, key string) (int, bool) {
	value, ok := parsePDFRawValue(dict, key, `(\d+)`)
	if !ok {
		return 0, false
	}
	parsed, _ := strconv.Atoi(value)
	return parsed, true
}

func parsePDFRawValue(dict []byte, key, pattern string) (string, bool) {
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s+(` + pattern + `)`)
	match := re.FindSubmatch(dict)
	if len(match) == 0 {
		return "", false
	}
	return string(match[1]), true
}

func replaceOrAddPDFString(dict []byte, key, value string) []byte {
	escaped := escapePDFString(value)
	replacement := []byte(fmt.Sprintf("/%s (%s)", key, escaped))
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s*\((?:\\.|[^\\)])*\)`)
	if re.Match(dict) {
		return re.ReplaceAll(dict, replacement)
	}
	return insertIntoPDFDict(dict, replacement)
}

func replaceOrAddPDFInt(dict []byte, key string, value int) []byte {
	replacement := []byte(fmt.Sprintf("/%s %d", key, value))
	re := regexp.MustCompile(`/` + regexp.QuoteMeta(key) + `\s+\d+`)
	if re.Match(dict) {
		return re.ReplaceAll(dict, replacement)
	}
	return insertIntoPDFDict(dict, replacement)
}

func insertIntoPDFDict(dict, entry []byte) []byte {
	idx := bytes.LastIndex(dict, []byte(">>"))
	if idx < 0 {
		return dict
	}
	var out bytes.Buffer
	out.Write(dict[:idx])
	out.WriteString("\n  ")
	out.Write(entry)
	out.WriteString("\n")
	out.Write(dict[idx:])
	return out.Bytes()
}

func replaceOrAddXMLTag(doc []byte, tag, value string) ([]byte, error) {
	escaped := escapeXMLText(value)
	replacement := []byte(fmt.Sprintf("<%s>%s</%s>", tag, escaped, tag))
	re := regexp.MustCompile(`(?s)<` + regexp.QuoteMeta(tag) + `>.*?</` + regexp.QuoteMeta(tag) + `>`)
	if re.Match(doc) {
		return re.ReplaceAll(doc, replacement), nil
	}
	idx := bytes.Index(doc, []byte("</rdf:Description>"))
	if idx < 0 {
		return nil, fmt.Errorf("xmp rdf:Description not found")
	}
	var out bytes.Buffer
	out.Write(doc[:idx])
	out.Write(replacement)
	out.Write(doc[idx:])
	return out.Bytes(), nil
}

func escapePDFString(value string) string {
	var out strings.Builder
	for _, r := range value {
		switch r {
		case '\\', '(', ')':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func escapeXMLText(value string) string {
	var out bytes.Buffer
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

func skipPDFWhitespace(pdf []byte, index int) int {
	for index < len(pdf) {
		switch pdf[index] {
		case ' ', '\t', '\n', '\r', '\f', 0:
			index++
		default:
			return index
		}
	}
	return index
}

func readPDFLine(pdf []byte, start int) ([]byte, int, error) {
	if start >= len(pdf) {
		return nil, start, io.EOF
	}
	end := start
	for end < len(pdf) && pdf[end] != '\n' && pdf[end] != '\r' {
		end++
	}
	next := end
	if next < len(pdf) && pdf[next] == '\r' {
		next++
	}
	if next < len(pdf) && pdf[next] == '\n' {
		next++
	}
	return pdf[start:end], next, nil
}
