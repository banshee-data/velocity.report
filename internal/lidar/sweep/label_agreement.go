package sweep

// LabellerAgreement summarises inter-labeller agreement for a set of tracks.
type LabellerAgreement struct {
	TotalTracks        int     `json:"total_tracks"`
	MultiLabelledCount int     `json:"multi_labelled_count"` // tracks with >1 labeller
	AgreementRate      float64 `json:"agreement_rate"`       // fraction of multi-labelled tracks with consensus
	DisagreementCount  int     `json:"disagreement_count"`   // tracks where labellers disagree
}

// LabelRecord represents a single label entry for agreement checking.
type LabelRecord struct {
	TrackID    string
	Label      string
	LabellerID string
}

// ComputeAgreement calculates labeller agreement from a set of label records.
// If tracks have multiple labels from different labellers, it checks if they agree.
func ComputeAgreement(labels []LabelRecord) LabellerAgreement {
	if len(labels) == 0 {
		return LabellerAgreement{}
	}

	// Group labels by TrackID
	trackLabels := make(map[string][]LabelRecord)
	for _, label := range labels {
		trackLabels[label.TrackID] = append(trackLabels[label.TrackID], label)
	}

	agreement := LabellerAgreement{
		TotalTracks: len(trackLabels),
	}

	// Check agreement for tracks with multiple labellers
	for _, records := range trackLabels {
		// Count distinct labellers for this track
		labellers := make(map[string]bool)
		for _, record := range records {
			labellers[record.LabellerID] = true
		}

		// Skip tracks with only one labeller
		if len(labellers) <= 1 {
			continue
		}

		agreement.MultiLabelledCount++

		// Check if all labels agree
		firstLabel := records[0].Label
		allAgree := true
		for _, record := range records[1:] {
			if record.Label != firstLabel {
				allAgree = false
				break
			}
		}

		if !allAgree {
			agreement.DisagreementCount++
		}
	}

	// Calculate agreement rate
	if agreement.MultiLabelledCount > 0 {
		agreementCount := agreement.MultiLabelledCount - agreement.DisagreementCount
		agreement.AgreementRate = float64(agreementCount) / float64(agreement.MultiLabelledCount)
	}

	return agreement
}
