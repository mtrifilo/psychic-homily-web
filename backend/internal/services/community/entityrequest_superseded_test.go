package community

import (
	communitym "psychic-homily-backend/internal/models/community"
)

// PSY-1978: a replacement overwrites payload, source_context and source_detail
// together, so the row alone cannot say it was ever anything else. Two things
// close that: CreateRequest hands the superseded submission back to the caller
// that will audit it, and the row records what it was ORIGINALLY filed as.

// aiSourceDetail is the source article an ai_extraction submission carries — the
// evidence a resubmission as 'manual' silently drops.
func aiSourceDetail() []byte {
	return []byte(`{"url":"https://example.com/tour-announced"}`)
}

// The ticket's exploit: file with AI provenance and a source article, resubmit
// as manual with neither. The live row is now indistinguishable from a plain
// manual request, so both records of the earlier claim have to survive.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_ReplacementReportsWhatItSuperseded() {
	user := suite.createUser("provenance", tierContributor, false)

	filed, superseded, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Sourced Band"), communitym.EntityRequestSourceAIExtraction,
		aiSourceDetail(), false)
	suite.Require().NoError(err)
	suite.Require().Nil(superseded)
	suite.Require().Nil(suite.requireStored(filed.ID).OriginalSourceContext,
		"a first filing has superseded nothing")

	replaced, superseded, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Sourced Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().NotNil(superseded, "a resubmission on the dedup key replaces the queued row")
	suite.Assert().Equal(filed.ID, replaced.ID)

	// What the caller is handed, which is the only place the destroyed
	// submission exists once the UPDATE has committed.
	suite.Assert().Equal(communitym.EntityRequestSourceAIExtraction, superseded.SourceContext)
	suite.Require().NotNil(superseded.Payload)
	suite.Assert().JSONEq(`{"name":"Sourced Band"}`, string(*superseded.Payload))
	suite.Require().NotNil(superseded.SourceDetail)
	suite.Assert().JSONEq(string(aiSourceDetail()), string(*superseded.SourceDetail))

	// What the ROW now says, which is what the moderation card reads.
	stored := suite.requireStored(filed.ID)
	suite.Assert().Equal(communitym.EntityRequestSourceManual, stored.SourceContext,
		"the live source_context is still the resubmission's, per PSY-1948")
	suite.Assert().Nil(stored.SourceDetail, "a resubmission with no detail clears the stored one")
	suite.Require().NotNil(stored.OriginalSourceContext)
	suite.Assert().Equal(communitym.EntityRequestSourceAIExtraction, *stored.OriginalSourceContext,
		"the row records that AI provenance was dropped")

	// The returned row agrees with the stored one, so a caller that reads it
	// instead of re-reading is not told something else.
	suite.Require().NotNil(replaced.OriginalSourceContext)
	suite.Assert().Equal(communitym.EntityRequestSourceAIExtraction, *replaced.OriginalSourceContext)
}

// Repeated replacement must keep naming the ORIGINAL filing. Recording the
// PREVIOUS one instead would let a requester launder AI provenance away in two
// resubmissions: ai_extraction to manual to manual leaves the column saying
// 'manual', which equals the live value and shows nothing.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_RepeatedReplacementKeepsTheOriginalSource() {
	user := suite.createUser("launder", tierContributor, false)

	filed, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Laundered Band"), communitym.EntityRequestSourceAIExtraction,
		aiSourceDetail(), false)
	suite.Require().NoError(err)

	for _, source := range []string{
		communitym.EntityRequestSourceManual,
		communitym.EntityRequestSourcePasteMode,
		communitym.EntityRequestSourceManual,
	} {
		_, superseded, rerr := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
			suite.marshalArtist("Laundered Band"), source, nil, false)
		suite.Require().NoError(rerr)
		suite.Require().NotNil(superseded)
	}

	stored := suite.requireStored(filed.ID)
	suite.Require().NotNil(stored.OriginalSourceContext)
	suite.Assert().Equal(communitym.EntityRequestSourceAIExtraction, *stored.OriginalSourceContext,
		"three replacements later, the row still names what was originally filed")
	suite.Assert().Equal(communitym.EntityRequestSourceManual, stored.SourceContext)
}

// A resubmission that does not change the source_context still records it, so
// the column is about REVISION rather than about downgrade. The moderation card
// is what decides whether an unchanged value is worth showing.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_ReplacementWithTheSameSourceStillRecordsIt() {
	user := suite.createUser("same-source", tierContributor, false)

	filed, _, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Steady Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)

	_, superseded, err := suite.service.CreateRequest(user, communitym.EntityRequestArtist,
		suite.marshalArtist("Steady Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Require().NotNil(superseded)

	stored := suite.requireStored(filed.ID)
	suite.Require().NotNil(stored.OriginalSourceContext)
	suite.Assert().Equal(communitym.EntityRequestSourceManual, *stored.OriginalSourceContext)
}

// An auto-approving tier never collides with the pending-only dedup index, so it
// files a second row rather than replacing the first — and neither row acquires
// an original_source_context.
func (suite *EntityRequestServiceIntegrationTestSuite) TestCreate_AutoApprovedFilingSupersedesNothing() {
	admin := suite.createUser("auto-super", tierNewUser, true)

	first, superseded, err := suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("Auto Band"), communitym.EntityRequestSourceAIExtraction, nil, false)
	suite.Require().NoError(err)
	suite.Require().Nil(superseded)

	second, superseded, err := suite.service.CreateRequest(admin, communitym.EntityRequestArtist,
		suite.marshalArtist("Auto Band"), communitym.EntityRequestSourceManual, nil, false)
	suite.Require().NoError(err)
	suite.Assert().Nil(superseded, "an approved row never meets the pending-only dedup index")
	suite.Assert().NotEqual(first.ID, second.ID)
	suite.Assert().Nil(suite.requireStored(second.ID).OriginalSourceContext)
}
