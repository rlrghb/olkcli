package graphapi

func getNextLink(resp interface{}) string {
	if r, ok := resp.(interface{ GetOdataNextLink() *string }); ok {
		if link := r.GetOdataNextLink(); link != nil {
			return *link
		}
	}
	return ""
}
