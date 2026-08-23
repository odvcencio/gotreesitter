CENSUS ?= full
TAGS := gts_parsercorephase0

.PHONY: census
census:
	GTS_ADMISSION_SCORECARD=1 GTS_ADMISSION_CENSUS=1 \
	go test -tags $(TAGS) -run TestAdmissionCandidateScorecard206 -v .

clean:
	rm -rf campaign/out
