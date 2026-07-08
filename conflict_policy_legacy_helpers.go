package gotreesitter

// dartRepetitionShiftConflictChoice resolves known Dart repeat-boundary forks.
func dartRepetitionShiftConflictChoice(lang *Language, state StateID, actions []ParseAction) (ParseAction, bool) {
	if lang == nil {
		return ParseAction{}, false
	}
	switch state {
	case 596:
		if !allReducesHaveSymbol(lang, actions, "enum_body_repeat2") {
			return ParseAction{}, false
		}
	case 602:
		if !allReducesHaveSymbol(lang, actions, "extension_body_repeat1") {
			return ParseAction{}, false
		}
	case 479:
		if !allReducesHaveSymbol(lang, actions, "program_repeat4") {
			return ParseAction{}, false
		}
	default:
		return ParseAction{}, false
	}
	return repetitionShiftConflictChoice(actions)
}

// erlangMacroCallExprConflictChoice resolves macro invocation forks in favor of the C-faithful args shift.
func erlangMacroCallExprConflictChoice(lang *Language, actions []ParseAction) (ParseAction, bool) {
	if lang == nil || len(actions) < 2 {
		return ParseAction{}, false
	}
	var shift ParseAction
	haveShift := false
	haveReduce := false
	for _, act := range actions {
		switch act.Type {
		case ParseActionShift:
			if haveShift || act.Repetition {
				return ParseAction{}, false
			}
			shift = act
			haveShift = true
		case ParseActionReduce:
			if act.ChildCount != 2 || !symbolHasName(lang, act.Symbol, "macro_call_expr") {
				return ParseAction{}, false
			}
			haveReduce = true
		default:
			return ParseAction{}, false
		}
	}
	if !haveShift || !haveReduce {
		return ParseAction{}, false
	}
	return shift, true
}
