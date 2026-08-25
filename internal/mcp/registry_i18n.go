package mcp

// The eleven, declared once.
//
// This is the proof that the registry shape carries real capabilities: the
// three actions routed for v3 are declared here and nothing else names them.
// The router dispatches from this, help answers from this, the advertised set
// counts from this, and the parameter names in the documentation are the names
// the handlers read — because they are the same declaration.
//
// Before this file the same eleven needed: an entry in a routing map, a case in
// the help switch, a mention in the tool description's action line, and a name
// in the sweep's advertised set. Four places, three of which were found out of
// step during one week.

// Parameter declarations. The tag is the name the handler reads; a mismatch is
// a compile-time-adjacent bug rather than a documentation one, because the same
// string appears in both places and can be grepped.
type (
	i18nTextsParams struct {
		ObjectURL string `vsp:"object_url,required,ADT path of the object"`
		Language  string `vsp:"language,required,two-letter code — EN, DE"`
	}
	i18nNameLangParams struct {
		Name     string `vsp:"name,required,object name"`
		Language string `vsp:"language,required,two-letter code"`
	}
	i18nCompareParams struct {
		ObjectURL string `vsp:"object_url,required,ADT path of the object"`
		Languages string `vsp:"languages,required,comma-separated — EN,DE"`
	}
	i18nWriteLabelsParams struct {
		Name     string `vsp:"name,required,data element name"`
		Language string `vsp:"language,required,two-letter code"`
		Short    string `vsp:"short,short label"`
		Medium   string `vsp:"medium,medium label"`
		Long     string `vsp:"long,long label"`
		Heading  string `vsp:"heading,column heading"`
	}
	i18nWriteMsgParams struct {
		Name     string `vsp:"name,required,message class"`
		Language string `vsp:"language,required,two-letter code"`
		Messages string `vsp:"messages,required,the texts to write"`
	}
	revisionListParams struct {
		ObjectType string `vsp:"object_type,required,CLAS, PROG, INTF, FUGR"`
		ObjectName string `vsp:"object_name,required,object name"`
	}
	revisionSourceParams struct {
		VersionURI string `vsp:"version_uri,required,taken from the list — not constructable by hand"`
	}
	revisionCompareParams struct {
		ObjectType string `vsp:"object_type,required,CLAS, PROG, INTF, FUGR"`
		ObjectName string `vsp:"object_name,required,object name"`
		From       string `vsp:"from,required,version to compare from"`
		To         string `vsp:"to,required,version to compare to"`
	}
	lintParams struct {
		Source     string `vsp:"source,ABAP source to analyse offline"`
		ObjectType string `vsp:"object_type,read the source from SAP instead"`
		ObjectName string `vsp:"object_name,with object_type"`
	}
)

// registerEleven declares the capabilities routed for v3.
func (s *Server) registerEleven() []Capability {
	return []Capability{
		{Action: "i18n", Op: "texts", Summary: "read an object's texts in one language",
			Params: i18nTextsParams{}, Handler: s.handleGetObjectTextsInLanguage,
			Examples: []string{`SAP(action="i18n", params={"op": "texts", "object_url": "/sap/bc/adt/oo/classes/zcl_demo", "language": "DE"})`}},
		{Action: "i18n", Op: "data_element_labels", Summary: "read a data element's short, medium, long and heading labels",
			Params: i18nNameLangParams{}, Handler: s.handleGetDataElementLabels,
			Examples: []string{`SAP(action="i18n", params={"op": "data_element_labels", "name": "ZDE_ORDER_ID", "language": "DE"})`}},
		{Action: "i18n", Op: "message_class_texts", Summary: "read the texts of a message class",
			Params: i18nNameLangParams{}, Handler: s.handleGetMessageClassTexts,
			Examples: []string{`SAP(action="i18n", params={"op": "message_class_texts", "name": "ZVSP_GIT", "language": "EN"})`}},
		{Action: "i18n", Op: "text_pool", Summary: "read a report's selection texts and text symbols",
			Params: i18nNameLangParams{}, Handler: s.handleGetTextPoolInLanguage,
			Examples: []string{`SAP(action="i18n", params={"op": "text_pool", "name": "ZDEMO_REPORT", "language": "EN"})`}},
		{Action: "i18n", Op: "compare_languages", Summary: "report what differs between two languages",
			Params: i18nCompareParams{}, Handler: s.handleCompareObjectLanguages,
			Examples: []string{`SAP(action="i18n", params={"op": "compare_languages", "object_url": "/sap/bc/adt/oo/classes/zcl_demo", "languages": "EN,DE"})`}},
		{Action: "i18n", Op: "write_labels", Summary: "write a data element's labels", Writes: true,
			Params: i18nWriteLabelsParams{}, Handler: s.handleWriteDataElementLabels,
			Examples: []string{`SAP(action="i18n", params={"op": "write_labels", "name": "ZDE_ORDER_ID", "language": "DE", "short": "Auftrag"})`}},
		{Action: "i18n", Op: "write_message_texts", Summary: "write the texts of a message class", Writes: true,
			Params: i18nWriteMsgParams{}, Handler: s.handleWriteMessageClassTexts,
			Examples: []string{`SAP(action="i18n", params={"op": "write_message_texts", "name": "ZVSP_GIT", "language": "DE", "messages": "..."})`}},

		{Action: "revisions", Op: "list", Summary: "list an object's versions",
			Params: revisionListParams{}, Handler: s.handleGetRevisions,
			Examples: []string{`SAP(action="revisions", params={"object_type": "CLAS", "object_name": "ZCL_DEMO"})`}},
		{Action: "revisions", Op: "source", Summary: "read one version's source",
			Params: revisionSourceParams{}, Handler: s.handleGetRevisionSource,
			Examples: []string{`SAP(action="revisions", params={"op": "source", "version_uri": "…from the list…"})`}},
		{Action: "revisions", Op: "compare", Summary: "compare two versions of an object",
			Params: revisionCompareParams{}, Handler: s.handleCompareVersions,
			Examples: []string{`SAP(action="revisions", params={"op": "compare", "object_type": "CLAS", "object_name": "ZCL_DEMO", "from": "…", "to": "…"})`}},

		{Action: "lint", Summary: "static analysis, offline — 13 rules, 8 on by default",
			Params: lintParams{}, Handler: s.handleAnalyzeABAPCode,
			Examples: []string{
				`SAP(action="lint", params={"source": "REPORT zdemo."})`,
				`SAP(action="lint", params={"object_type": "CLAS", "object_name": "ZCL_DEMO"})`,
			}},
	}
}
