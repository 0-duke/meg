package main

import (
	"github.com/deckarep/golang-set"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/antchfx/htmlquery"
	"bytes"
	"strings"
	"golang.org/x/net/html"
	"regexp"
	"fmt"
)

// JaccardSimilarity, as known as the Jaccard Index, compares the similarity of sample sets.
// This doesn't measure similarity between texts, but if regarding a text as bag-of-word,
// it can apply.
func JaccardSimilarity(s1, s2 string, f func(string) mapset.Set) float64 {
	if s1 == s2 {
		return 1.0
	}
	if f == nil {
		f = convertStringToSet
	}
	s1set := f(s1)
	s2set := f(s2)
	s1ands2 := s1set.Intersect(s2set).Cardinality()
	s1ors2 := s1set.Union(s2set).Cardinality()
	return float64(s1ands2) / float64(s1ors2)
}

func convertStringToSet(s string) mapset.Set {
	set := mapset.NewSet()
	for _, token := range strings.Fields(s) {
		set.Add(token)
	}
	return set
}

func sequenceMatcher(a,b []string) float64 {
	res := difflib.NewMatcher(a, b)
	ratio := res.Ratio()
	return ratio
}

// Create a structure (<html><body>..) even if the content is empty
func getAllTags(html_content []byte) []string {
	//doc, err := htmlquery.LoadURL(html)
	doc, err := htmlquery.Parse(bytes.NewReader(html_content))
	if err != nil {
		panic(err)
	}
	tags := []string{}
	for _, e  := range htmlquery.Find(doc, "//*"){
			tags = append(tags, e.Data)
	}
	return tags
}

func getTagsTokenizer(html_content []byte) []string {
	z := html.NewTokenizer(bytes.NewReader(html_content))
	tags := []string{}
    for {
		tt := z.Next()
		switch tt {
			case html.ErrorToken:
				return tags
			/*case html.TextToken:
				tags = append(tags, "text")*/
			case html.StartTagToken:
				tn, _ := z.TagName()
				if(len(tn) > 0) {
					tags = append(tags, fmt.Sprintf("%s", tn))
				}
		}		
		
	}	
	return tags

}

func getAllClasses(html_content []byte) string {
	//doc, err := htmlquery.LoadURL(html)
	doc, err := htmlquery.Parse(bytes.NewReader(html_content))
	if err != nil {
		panic(err)
	}
	classes := []string{}
	for _, e  := range htmlquery.Find(doc, "//*[@class]/@class"){
		classes = append(classes, htmlquery.SelectAttr(e, "class"))		
	}
	return strings.Join(classes, " ")
}



func structuralSimilarity(html1,html2 []byte) float64 {
	// To Change with HTML body
	r := sequenceMatcher(getAllTags(html1), getAllTags(html2))
	return r
}

func styleSimilarity(html1, html2 []byte) float64 {
	// To Change with HTML body
	r := JaccardSimilarity(getAllClasses(html1), getAllClasses(html2), nil)
	return r
}

func similarity(html1, html2 []byte) float64 {
	k := 0.3
	structS := structuralSimilarity(html1, html2)
	styleS := styleSimilarity(html1, html2)
	return k * structS + (1 - k) * styleS
}

func excludeHTTPHeaders(html []byte) []byte {
	//We use "< HTTP/1.1" since meg is using this fixed string to created the output
	var httpResponseHeaderRegExp = `< HTTP\/1.1(.*\n)+< .+?\n\n`
	httpRespHeader := regexp.MustCompile(httpResponseHeaderRegExp)
	indexes := httpRespHeader.FindAllIndex([]byte(html), -1)

	// If there is one match it's OK and proceed to extract the HTTP Body
	// Otherwise there is an error and it's better to return the input as it is.
	if (len(indexes) == 1){
		for _, j := range indexes {
			// j[0] will always contain the starting position of the match
			// j[1] will always contain the ending position of the match
			// we use the latter
			return html[j[1]:]
		}
	} 
	
	return html
}
