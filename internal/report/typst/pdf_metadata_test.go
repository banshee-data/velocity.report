package typst

import (
	"bytes"
	"strings"
	"testing"
)

func buildTestPDF(objects []string, trailer string) []byte {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for index, obj := range objects {
		offsets[index+1] = pdf.Len()
		pdf.WriteString(obj)
	}
	xrefStart := pdf.Len()
	pdf.WriteString("xref\n")
	pdf.WriteString("0 ")
	pdf.WriteString(string('0' + byte(len(objects)+1)))
	pdf.WriteString("\n")
	pdf.WriteString("0000000000 65535 f \n")
	for index := 1; index <= len(objects); index++ {
		pdf.WriteString(strings.TrimSpace(string(formatXRefEntry(offsets[index]))))
		pdf.WriteByte('\n')
	}
	pdf.WriteString("trailer\n")
	pdf.WriteString(trailer)
	pdf.WriteString("\nstartxref\n")
	pdf.WriteString(stringInt(xrefStart))
	pdf.WriteString("\n%%EOF\n")
	return pdf.Bytes()
}

func formatXRefEntry(offset int) []byte {
	return []byte(padInt(offset) + " 00000 n ")
}

func padInt(value int) string {
	text := stringInt(value)
	for len(text) < 10 {
		text = "0" + text
	}
	return text
}

func stringInt(value int) string {
	if value == 0 {
		return "0"
	}
	buf := [20]byte{}
	idx := len(buf)
	for value > 0 {
		idx--
		buf[idx] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[idx:])
}

func testPDFWithoutInfoOrMetadata() []byte {
	return buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
		},
		"<< /Size 3 /Root 1 0 R >>",
	)
}

func TestApplyPDFMetadataEmptyReturnsInput(t *testing.T) {
	pdf := testMetadataFixturePDF()
	updated, err := applyPDFMetadata(pdf, PDFMetadata{})
	if err != nil {
		t.Fatalf("applyPDFMetadata: %v", err)
	}
	if !bytes.Equal(updated, pdf) {
		t.Fatal("empty PDF metadata should leave the PDF unchanged")
	}
}

func TestApplyPDFMetadataAddsInfoWhenMissing(t *testing.T) {
	updated, err := applyPDFMetadata(testPDFWithoutInfoOrMetadata(), PDFMetadata{
		Creator:  "velocity.report v9.9.9",
		Keywords: []string{"git-sha:abcdef"},
	})
	if err != nil {
		t.Fatalf("applyPDFMetadata: %v", err)
	}
	for _, want := range []string{
		"/Creator (velocity.report v9.9.9)",
		"/Keywords (git-sha:abcdef)",
		"/Info 3 0 R",
		"/Prev ",
	} {
		if !bytes.Contains(updated, []byte(want)) {
			t.Fatalf("updated pdf missing %q", want)
		}
	}
}

func TestApplyPDFMetadataReportsMissingReferencedObjects(t *testing.T) {
	infoMissing := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
		},
		"<< /Size 4 /Root 1 0 R /Info 3 0 R >>",
	)
	if _, err := applyPDFMetadata(infoMissing, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "info object 3 missing") {
		t.Fatalf("missing info error = %v, want info object missing", err)
	}

	rootMissing := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
		},
		"<< /Size 3 /Root 5 0 R >>",
	)
	if _, err := applyPDFMetadata(rootMissing, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "catalog object 5 missing") {
		t.Fatalf("missing root error = %v, want catalog object missing", err)
	}

	metadataMissing := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
			"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
		},
		"<< /Size 5 /Root 1 0 R /Info 3 0 R >>",
	)
	if _, err := applyPDFMetadata(metadataMissing, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "metadata object 4 missing") {
		t.Fatalf("missing metadata error = %v, want metadata object missing", err)
	}

	for _, tc := range []struct {
		name string
		pdf  []byte
	}{
		{name: "missing startxref", pdf: []byte("%PDF-1.7\n")},
		{name: "missing startxref value", pdf: []byte("startxref")},
		{name: "bad startxref", pdf: []byte("startxref\nabc\n")},
		{name: "missing trailer", pdf: []byte("startxref\n0\n")},
		{name: "bad trailer dictionary", pdf: buildTestPDF([]string{"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n", "2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n"}, "<< /Info 3 0 R >>")},
		{name: "bad xref", pdf: []byte("%PDF-1.7\nxref\n0 1\nnope\ntrailer\n<< /Size 3 /Root 1 0 R >>\nstartxref\n9\n%%EOF\n")},
		{name: "unsupported xref stream", pdf: []byte("startxref\n0\nstream")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := applyPDFMetadata(tc.pdf, PDFMetadata{Creator: "x"}); err == nil {
				t.Fatalf("applyPDFMetadata should fail for %s", tc.name)
			}
		})
	}
}

func TestApplyPDFMetadataPropagatesObjectReadAndUpdateErrors(t *testing.T) {
	infoReadFailure := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
			"3 0 obj\n<< /Creator (Typst 0.13.1) >>\n",
		},
		"<< /Size 4 /Root 1 0 R /Info 3 0 R >>",
	)
	if _, err := applyPDFMetadata(infoReadFailure, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "missing endobj") {
		t.Fatalf("info read error = %v, want missing endobj", err)
	}

	infoUpdateFailure := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
			"3 0 obj\nno-dict\nendobj\n",
		},
		"<< /Size 4 /Root 1 0 R /Info 3 0 R >>",
	)
	if _, err := applyPDFMetadata(infoUpdateFailure, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "dictionary start") {
		t.Fatalf("info update error = %v, want dictionary start", err)
	}

	rootReadFailure := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
			"2 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
			"3 0 obj\n<< /Type /Catalog /Pages 1 0 R >>\n",
		},
		"<< /Size 4 /Root 3 0 R /Info 2 0 R >>",
	)
	if _, err := applyPDFMetadata(rootReadFailure, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "missing endobj") {
		t.Fatalf("root read error = %v, want missing endobj", err)
	}

	rootDictFailure := buildTestPDF(
		[]string{
			"1 0 obj\nno-dict\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
			"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
		},
		"<< /Size 4 /Root 1 0 R /Info 3 0 R >>",
	)
	if _, err := applyPDFMetadata(rootDictFailure, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "dictionary start") {
		t.Fatalf("root dict error = %v, want dictionary start", err)
	}

	metadataReadFailure := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
			"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
			"4 0 obj\n<< /Type /Metadata /Subtype /XML /Length 6 >>\nstream\n<xml/>\n",
		},
		"<< /Size 5 /Root 1 0 R /Info 3 0 R >>",
	)
	if _, err := applyPDFMetadata(metadataReadFailure, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "missing endobj") {
		t.Fatalf("metadata read error = %v, want missing endobj", err)
	}

	metadataUpdateFailure := buildTestPDF(
		[]string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
			"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
			"4 0 obj\n<< /Type /Metadata /Subtype /XML /Length 6 >>\nstream\n<xml/>\nendstream\nendobj\n",
		},
		"<< /Size 5 /Root 1 0 R /Info 3 0 R >>",
	)
	if _, err := applyPDFMetadata(metadataUpdateFailure, PDFMetadata{Creator: "x"}); err == nil || !strings.Contains(err.Error(), "rdf:Description") {
		t.Fatalf("metadata update error = %v, want rdf:Description", err)
	}
}

func TestPDFMetadataParsersAndObjectReaders(t *testing.T) {
	if _, err := parseStartXRef([]byte("%PDF-1.7\n")); err == nil {
		t.Fatal("parseStartXRef should fail when startxref is missing")
	}
	if _, err := parseStartXRef([]byte("startxref\nabc\n")); err == nil {
		t.Fatal("parseStartXRef should fail on a non-integer offset")
	}

	if _, err := parseLastTrailerDict([]byte("trailer\n<<>>\n")); err == nil {
		t.Fatal("parseLastTrailerDict should fail when startxref is missing")
	}
	if _, err := parseLastTrailerDict([]byte("%PDF-1.7\nstartxref\n0\n")); err == nil {
		t.Fatal("parseLastTrailerDict should fail when trailer is missing")
	}
	if _, err := parseLastTrailerDict([]byte("trailer\nstartxref\n0\n")); err == nil {
		t.Fatal("parseLastTrailerDict should fail when the trailer dictionary is missing")
	}
	if _, err := parseLastTrailerDict([]byte("trailer\n<<\nstartxref\n0\n")); err == nil {
		t.Fatal("parseLastTrailerDict should fail when the trailer dictionary is unterminated")
	}

	if _, err := parsePDFTrailer([]byte("<< /Root 1 0 R >>")); err == nil {
		t.Fatal("parsePDFTrailer should fail without Size")
	}
	if _, err := parsePDFTrailer([]byte("<< /Size 3 >>")); err == nil {
		t.Fatal("parsePDFTrailer should fail without Root")
	}

	if _, err := parseXRefTable([]byte("stream"), 0); err == nil || !strings.Contains(err.Error(), "xref stream") {
		t.Fatalf("parseXRefTable unsupported error = %v", err)
	}
	if _, err := parseXRefTable([]byte("xref"), 999); err == nil {
		t.Fatal("parseXRefTable should fail for an out-of-range xref offset")
	}
	if _, err := parseXRefTable([]byte("xref\n0 1\n"), 0); err == nil {
		t.Fatal("parseXRefTable should fail on an unterminated xref table")
	}
	if _, err := parseXRefTable([]byte("xref\nnope 1\ntrailer\n<<>>"), 0); err == nil {
		t.Fatal("parseXRefTable should fail on a non-integer subsection object number")
	}
	if _, err := parseXRefTable([]byte("xref\n0 nope\ntrailer\n<<>>"), 0); err == nil {
		t.Fatal("parseXRefTable should fail on a non-integer subsection count")
	}
	if _, err := parseXRefTable([]byte("xref\nnope\ntrailer\n<<>>"), 0); err == nil {
		t.Fatal("parseXRefTable should fail on a malformed subsection header")
	}
	if _, err := parseXRefTable([]byte("xref\n0 1\nnope\ntrailer\n<<>>"), 0); err == nil {
		t.Fatal("parseXRefTable should fail on a malformed entry")
	}
	if _, err := parseXRefTable([]byte("xref\n0 1\nnotanumber 00000 n \ntrailer\n<<>>"), 0); err == nil {
		t.Fatal("parseXRefTable should fail on a non-integer xref offset")
	}

	if _, err := readPDFObject([]byte(""), 1); err == nil {
		t.Fatal("readPDFObject should fail for an out-of-range offset")
	}
	if _, err := readPDFObject([]byte("1 0 obj\n"), 0); err == nil {
		t.Fatal("readPDFObject should fail without endobj")
	}

	if _, err := extractPDFDict([]byte("1 0 obj\n")); err == nil {
		t.Fatal("extractPDFDict should fail without a dictionary")
	}
	if _, err := extractPDFDict([]byte("1 0 obj\n<<\nendobj")); err == nil {
		t.Fatal("extractPDFDict should fail without a closing >>")
	}
	if _, _, err := extractPDFStreamObject([]byte("stream\nbody\nendstream")); err == nil {
		t.Fatal("extractPDFStreamObject should fail without a dictionary")
	}
	if _, _, err := extractPDFStreamObject([]byte("<<\nstream\nbody\nendstream")); err == nil {
		t.Fatal("extractPDFStreamObject should fail without a closing dictionary")
	}
	if _, _, err := extractPDFStreamObject([]byte("<<>>")); err == nil {
		t.Fatal("extractPDFStreamObject should fail without a stream")
	}
	if _, _, err := extractPDFStreamObject([]byte("<<>>\nstream\nhello")); err == nil {
		t.Fatal("extractPDFStreamObject should fail without endstream")
	}
}

func TestPDFMetadataValueAndMutationHelpers(t *testing.T) {
	ref, ok := parseIndirectRef([]byte("<< /Info 3 0 R >>"), "Info")
	if !ok || ref != (pdfRef{number: 3, generation: 0}) {
		t.Fatalf("parseIndirectRef = %+v, ok=%v", ref, ok)
	}
	if _, ok := parseIndirectRef([]byte("<<>>"), "Info"); ok {
		t.Fatalf("parseIndirectRef missing = ok:%v, want false", ok)
	}

	value, ok := parsePDFInt([]byte("<< /Size 17 >>"), "Size")
	if !ok || value != 17 {
		t.Fatalf("parsePDFInt = %d, ok=%v", value, ok)
	}
	if _, ok := parsePDFInt([]byte("<<>>"), "Size"); ok {
		t.Fatalf("parsePDFInt missing = ok:%v, want false", ok)
	}

	raw, ok := parsePDFRawValue([]byte("<< /ID [<abc> <def>] >>"), "ID", `\[[^\]]+\]`)
	if !ok || raw != "[<abc> <def>]" {
		t.Fatalf("parsePDFRawValue = %q, ok=%v", raw, ok)
	}
	if _, ok := parsePDFRawValue([]byte("<<>>"), "ID", `\[[^\]]+\]`); ok {
		t.Fatalf("parsePDFRawValue missing = ok:%v, want false", ok)
	}

	if got := replaceOrAddPDFString([]byte("<< /Creator (Typst 0.13.1) >>"), "Creator", "velocity.report v1.2.3"); !bytes.Contains(got, []byte("/Creator (velocity.report v1.2.3)")) {
		t.Fatalf("replaceOrAddPDFString replace = %q", got)
	}
	if got := replaceOrAddPDFString([]byte("<< /Type /Info >>"), "Creator", "velocity.report v1.2.3"); !bytes.Contains(got, []byte("/Creator (velocity.report v1.2.3)")) {
		t.Fatalf("replaceOrAddPDFString add = %q", got)
	}
	if got := replaceOrAddPDFInt([]byte("<< /Length 7 >>"), "Length", 9); !bytes.Contains(got, []byte("/Length 9")) {
		t.Fatalf("replaceOrAddPDFInt replace = %q", got)
	}
	if got := replaceOrAddPDFInt([]byte("<< /Type /Metadata >>"), "Length", 9); !bytes.Contains(got, []byte("/Length 9")) {
		t.Fatalf("replaceOrAddPDFInt add = %q", got)
	}
	if got := insertIntoPDFDict([]byte("not-a-dict"), []byte("/Length 9")); string(got) != "not-a-dict" {
		t.Fatalf("insertIntoPDFDict = %q, want unchanged", got)
	}

	xmlDoc := []byte(`<rdf:Description><xmp:CreatorTool>Typst 0.13.1</xmp:CreatorTool></rdf:Description>`)
	updatedXML, err := replaceOrAddXMLTag(xmlDoc, "xmp:CreatorTool", "velocity & report")
	if err != nil || !bytes.Contains(updatedXML, []byte("<xmp:CreatorTool>velocity &amp; report</xmp:CreatorTool>")) {
		t.Fatalf("replaceOrAddXMLTag replace = %q, err=%v", updatedXML, err)
	}
	updatedXML, err = replaceOrAddXMLTag([]byte(`<rdf:Description></rdf:Description>`), "pdf:Keywords", "git-sha:abc")
	if err != nil || !bytes.Contains(updatedXML, []byte("<pdf:Keywords>git-sha:abc</pdf:Keywords>")) {
		t.Fatalf("replaceOrAddXMLTag add = %q, err=%v", updatedXML, err)
	}
	if _, err := replaceOrAddXMLTag([]byte(`<xml/>`), "pdf:Keywords", "git-sha:abc"); err == nil {
		t.Fatal("replaceOrAddXMLTag should fail without rdf:Description")
	}

	if got := escapePDFString("(a)\\\n\r\t"); got != `\(a\)\\\n\r\t` {
		t.Fatalf("escapePDFString = %q", got)
	}
	if got := escapeXMLText(`5 < 6 & 7`); got != "5 &lt; 6 &amp; 7" {
		t.Fatalf("escapeXMLText = %q", got)
	}

	if skipPDFWhitespace([]byte(" \t\n\r\f\000abc"), 0) != 6 {
		t.Fatal("skipPDFWhitespace returned the wrong index")
	}
	if skipPDFWhitespace([]byte("abc"), 0) != 0 {
		t.Fatal("skipPDFWhitespace should leave a non-whitespace prefix untouched")
	}
	if skipPDFWhitespace([]byte(" \t"), 0) != 2 {
		t.Fatal("skipPDFWhitespace should advance to the end of an all-whitespace buffer")
	}
	line, next, err := readPDFLine([]byte("abc\r\ndef"), 0)
	if err != nil || string(line) != "abc" || next != 5 {
		t.Fatalf("readPDFLine = %q next=%d err=%v", line, next, err)
	}
	if _, _, err := readPDFLine([]byte("abc"), 3); err == nil {
		t.Fatal("readPDFLine should return EOF at end of buffer")
	}
}

func TestUpdateInfoAndMetadataObjects(t *testing.T) {
	pdf := testMetadataFixturePDF()
	startXRef, err := parseStartXRef(pdf)
	if err != nil {
		t.Fatalf("parseStartXRef: %v", err)
	}
	xref, err := parseXRefTable(pdf, startXRef)
	if err != nil {
		t.Fatalf("parseXRefTable: %v", err)
	}
	infoObj, err := readPDFObject(pdf, xref[3])
	if err != nil {
		t.Fatalf("read info object: %v", err)
	}
	updatedInfo, err := updateInfoObject(infoObj, pdfRef{number: 3, generation: 0}, PDFMetadata{Creator: "velocity.report v1.2.3", Keywords: []string{"git-sha:abc123"}})
	if err != nil {
		t.Fatalf("updateInfoObject: %v", err)
	}
	if !bytes.Contains(updatedInfo, []byte("3 0 obj")) || !bytes.Contains(updatedInfo, []byte("/Keywords (git-sha:abc123)")) {
		t.Fatalf("updatedInfo = %q", updatedInfo)
	}

	metadataObj, err := readPDFObject(pdf, xref[4])
	if err != nil {
		t.Fatalf("read metadata object: %v", err)
	}
	updatedMetadata, err := updateMetadataObject(metadataObj, pdfRef{number: 4, generation: 0}, PDFMetadata{Creator: "velocity.report v1.2.3", Keywords: []string{"git-sha:abc123"}})
	if err != nil {
		t.Fatalf("updateMetadataObject: %v", err)
	}
	if !bytes.Contains(updatedMetadata, []byte("<pdf:Keywords>git-sha:abc123</pdf:Keywords>")) || !bytes.Contains(updatedMetadata, []byte("/Length ")) {
		t.Fatalf("updatedMetadata = %q", updatedMetadata)
	}

	if _, err := updateInfoObject([]byte("no-dict"), pdfRef{number: 1, generation: 0}, PDFMetadata{Creator: "x"}); err == nil {
		t.Fatal("updateInfoObject should fail without a dictionary")
	}
	if _, err := updateMetadataObject([]byte("no-stream"), pdfRef{number: 1, generation: 0}, PDFMetadata{Creator: "x"}); err == nil {
		t.Fatal("updateMetadataObject should fail without a stream")
	}
	badMetadata := []byte("4 0 obj\n<< /Type /Metadata /Subtype /XML /Length 6 >>\nstream\n<xml/>\nendstream\nendobj")
	if _, err := updateMetadataObject(badMetadata, pdfRef{number: 4, generation: 0}, PDFMetadata{Keywords: []string{"x"}}); err == nil {
		t.Fatal("updateMetadataObject should fail when rdf:Description is missing")
	}

	withCRLF := []byte("4 0 obj\n<< /Type /Metadata /Subtype /XML /Length 8 >>\nstream\r\n<body/>\nendstream\nendobj")
	if _, _, err := extractPDFStreamObject(withCRLF); err != nil {
		t.Fatalf("extractPDFStreamObject with CRLF: %v", err)
	}

	updated := appendIncrementalUpdate([]byte("%PDF-1.7\n"), pdfTrailer{size: 6, root: pdfRef{number: 1, generation: 0}, id: "[<abc> <def>]"}, 12, &pdfRef{number: 5, generation: 0}, map[int][]byte{5: buildIndirectObject(pdfRef{number: 5, generation: 0}, []byte("<<>>"))})
	if !bytes.Contains(updated, []byte("/ID [<abc> <def>]")) {
		t.Fatalf("appendIncrementalUpdate missing trailer ID: %q", updated)
	}
}
