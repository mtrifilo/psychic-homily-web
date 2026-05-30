package admin

import (
	"reflect"
	"testing"
)

func TestAdminPaginationQueryTagsAreBounded(t *testing.T) {
	tests := []struct {
		name      string
		request   any
		limitTag  string
		offsetTag string
	}{
		{
			name:      "pending shows",
			request:   GetPendingShowsRequest{},
			limitTag:  `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of shows to return (max 100)"`,
			offsetTag: `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "admin shows",
			request:   GetAdminShowsRequest{},
			limitTag:  `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of shows to return (max 100)"`,
			offsetTag: `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "unverified venues",
			request:   GetUnverifiedVenuesRequest{},
			limitTag:  `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of venues to return (max 100)"`,
			offsetTag: `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "entity revision history",
			request:   GetEntityHistoryRequest{},
			limitTag:  `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`,
			offsetTag: `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "user revisions",
			request:   GetUserRevisionsRequest{},
			limitTag:  `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`,
			offsetTag: `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "data quality category",
			request:   GetDataQualityCategoryRequest{},
			limitTag:  `query:"limit" required:"false" minimum:"1" maximum:"200" doc:"Max results (default 50, max 200)"`,
			offsetTag: `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "audit logs",
			request:   GetAuditLogsRequest{},
			limitTag:  `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Number of logs to return (max 100)"`,
			offsetTag: `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "export shows",
			request:   ExportShowsRequest{},
			limitTag:  `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Number of shows to return (max 200)"`,
			offsetTag: `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "my pending edits",
			request:   GetMyPendingEditsRequest{},
			limitTag:  `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`,
			offsetTag: `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`,
		},
		{
			name:      "admin pending edits",
			request:   AdminListPendingEditsRequest{},
			limitTag:  `query:"limit" required:"false" minimum:"1" maximum:"100" doc:"Max results (default 20, max 100)"`,
			offsetTag: `query:"offset" required:"false" minimum:"0" doc:"Offset for pagination"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestType := reflect.TypeOf(tt.request)

			limitField, ok := requestType.FieldByName("Limit")
			if !ok {
				t.Fatalf("%s is missing Limit field", requestType.Name())
			}
			if got := string(limitField.Tag); got != tt.limitTag {
				t.Fatalf("Limit tag mismatch:\ngot:  %s\nwant: %s", got, tt.limitTag)
			}

			offsetField, ok := requestType.FieldByName("Offset")
			if !ok {
				t.Fatalf("%s is missing Offset field", requestType.Name())
			}
			if got := string(offsetField.Tag); got != tt.offsetTag {
				t.Fatalf("Offset tag mismatch:\ngot:  %s\nwant: %s", got, tt.offsetTag)
			}
		})
	}
}
