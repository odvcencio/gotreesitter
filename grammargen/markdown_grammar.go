package grammargen

// MarkdownGrammar returns the Go-DSL definition of the CommonMark + GFM
// Markdown grammar. Equivalent in shape to the upstream tree-sitter-markdown
// grammar.json but owned in Go so it can be refactored, extended via
// ExtendGrammar, and compiled directly with GenerateLanguage(AndBlob).
//
// External scanner is NOT attached here. Callers must follow GenerateLanguage
// with `grammars.AdaptScannerForLanguage("markdown", lang)` to attach the
// hand-written external scanner that owns the 47 block/inline external tokens.
func MarkdownGrammar() *Grammar {
	g := NewGrammar("markdown")

	// top-level document: optional metadata, then sections/blocks
	g.Define("document",
		Seq(
			Choice(
				Choice(
					Sym("minus_metadata"),
					Sym("plus_metadata")),
				Blank()),
			Alias(PrecRight(0, Repeat(Sym("_block_not_section"))), "section", true),
			Repeat(Sym("section"))))

	// visible alias for the private _backslash_escape pattern
	g.Define("backslash_escape",
		Sym("_backslash_escape"))

	// a backslash followed by an ASCII punctuation character
	g.Define("_backslash_escape",
		Pat("\\\\[!-/:-@\\[-`\\{-~]"))

	// HTML named entity reference like &amp;
	g.Define("entity_reference",
		Pat(`&(AEli|AElig|AM|AMP|Aacut|Aacute|Abreve|Acir|Acirc|Acy|Afr|Agrav|Agrave|Alpha|Amacr|And|Aogon|Aopf|ApplyFunction|Arin|Aring|Ascr|Assign|Atild|Atilde|Aum|Auml|Backslash|Barv|Barwed|Bcy|Because|Bernoullis|Beta|Bfr|Bopf|Breve|Bscr|Bumpeq|CHcy|COP|COPY|Cacute|Cap|CapitalDifferentialD|Cayleys|Ccaron|Ccedi|Ccedil|Ccirc|Cconint|Cdot|Cedilla|CenterDot|Cfr|Chi|CircleDot|CircleMinus|CirclePlus|CircleTimes|ClockwiseContourIntegral|CloseCurlyDoubleQuote|CloseCurlyQuote|Colon|Colone|Congruent|Conint|ContourIntegral|Copf|Coproduct|CounterClockwiseContourIntegral|Cross|Cscr|Cup|CupCap|DD|DDotrahd|DJcy|DScy|DZcy|Dagger|Darr|Dashv|Dcaron|Dcy|Del|Delta|Dfr|DiacriticalAcute|DiacriticalDot|DiacriticalDoubleAcute|DiacriticalGrave|DiacriticalTilde|Diamond|DifferentialD|Dopf|Dot|DotDot|DotEqual|DoubleContourIntegral|DoubleDot|DoubleDownArrow|DoubleLeftArrow|DoubleLeftRightArrow|DoubleLeftTee|DoubleLongLeftArrow|DoubleLongLeftRightArrow|DoubleLongRightArrow|DoubleRightArrow|DoubleRightTee|DoubleUpArrow|DoubleUpDownArrow|DoubleVerticalBar|DownArrow|DownArrowBar|DownArrowUpArrow|DownBreve|DownLeftRightVector|DownLeftTeeVector|DownLeftVector|DownLeftVectorBar|DownRightTeeVector|DownRightVector|DownRightVectorBar|DownTee|DownTeeArrow|Downarrow|Dscr|Dstrok|ENG|ET|ETH|Eacut|Eacute|Ecaron|Ecir|Ecirc|Ecy|Edot|Efr|Egrav|Egrave|Element|Emacr|EmptySmallSquare|EmptyVerySmallSquare|Eogon|Eopf|Epsilon|Equal|EqualTilde|Equilibrium|Escr|Esim|Eta|Eum|Euml|Exists|ExponentialE|Fcy|Ffr|FilledSmallSquare|FilledVerySmallSquare|Fopf|ForAll|Fouriertrf|Fscr|GJcy|G|GT|Gamma|Gammad|Gbreve|Gcedil|Gcirc|Gcy|Gdot|Gfr|Gg|Gopf|GreaterEqual|GreaterEqualLess|GreaterFullEqual|GreaterGreater|GreaterLess|GreaterSlantEqual|GreaterTilde|Gscr|Gt|HARDcy|Hacek|Hat|Hcirc|Hfr|HilbertSpace|Hopf|HorizontalLine|Hscr|Hstrok|HumpDownHump|HumpEqual|IEcy|IJlig|IOcy|Iacut|Iacute|Icir|Icirc|Icy|Idot|Ifr|Igrav|Igrave|Im|Imacr|ImaginaryI|Implies|Int|Integral|Intersection|InvisibleComma|InvisibleTimes|Iogon|Iopf|Iota|Iscr|Itilde|Iukcy|Ium|Iuml|Jcirc|Jcy|Jfr|Jopf|Jscr|Jsercy|Jukcy|KHcy|KJcy|Kappa|Kcedil|Kcy|Kfr|Kopf|Kscr|LJcy|L|LT|Lacute|Lambda|Lang|Laplacetrf|Larr|Lcaron|Lcedil|Lcy|LeftAngleBracket|LeftArrow|LeftArrowBar|LeftArrowRightArrow|LeftCeiling|LeftDoubleBracket|LeftDownTeeVector|LeftDownVector|LeftDownVectorBar|LeftFloor|LeftRightArrow|LeftRightVector|LeftTee|LeftTeeArrow|LeftTeeVector|LeftTriangle|LeftTriangleBar|LeftTriangleEqual|LeftUpDownVector|LeftUpTeeVector|LeftUpVector|LeftUpVectorBar|LeftVector|LeftVectorBar|Leftarrow|Leftrightarrow|LessEqualGreater|LessFullEqual|LessGreater|LessLess|LessSlantEqual|LessTilde|Lfr|Ll|Lleftarrow|Lmidot|LongLeftArrow|LongLeftRightArrow|LongRightArrow|Longleftarrow|Longleftrightarrow|Longrightarrow|Lopf|LowerLeftArrow|LowerRightArrow|Lscr|Lsh|Lstrok|Lt|Map|Mcy|MediumSpace|Mellintrf|Mfr|MinusPlus|Mopf|Mscr|Mu|NJcy|Nacute|Ncaron|Ncedil|Ncy|NegativeMediumSpace|NegativeThickSpace|NegativeThinSpace|NegativeVeryThinSpace|NestedGreaterGreater|NestedLessLess|NewLine|Nfr|NoBreak|NonBreakingSpace|Nopf|Not|NotCongruent|NotCupCap|NotDoubleVerticalBar|NotElement|NotEqual|NotEqualTilde|NotExists|NotGreater|NotGreaterEqual|NotGreaterFullEqual|NotGreaterGreater|NotGreaterLess|NotGreaterSlantEqual|NotGreaterTilde|NotHumpDownHump|NotHumpEqual|NotLeftTriangle|NotLeftTriangleBar|NotLeftTriangleEqual|NotLess|NotLessEqual|NotLessGreater|NotLessLess|NotLessSlantEqual|NotLessTilde|NotNestedGreaterGreater|NotNestedLessLess|NotPrecedes|NotPrecedesEqual|NotPrecedesSlantEqual|NotReverseElement|NotRightTriangle|NotRightTriangleBar|NotRightTriangleEqual|NotSquareSubset|NotSquareSubsetEqual|NotSquareSuperset|NotSquareSupersetEqual|NotSubset|NotSubsetEqual|NotSucceeds|NotSucceedsEqual|NotSucceedsSlantEqual|NotSucceedsTilde|NotSuperset|NotSupersetEqual|NotTilde|NotTildeEqual|NotTildeFullEqual|NotTildeTilde|NotVerticalBar|Nscr|Ntild|Ntilde|Nu|OElig|Oacut|Oacute|Ocir|Ocirc|Ocy|Odblac|Ofr|Ograv|Ograve|Omacr|Omega|Omicron|Oopf|OpenCurlyDoubleQuote|OpenCurlyQuote|Or|Oscr|Oslas|Oslash|Otild|Otilde|Otimes|Oum|Ouml|OverBar|OverBrace|OverBracket|OverParenthesis|PartialD|Pcy|Pfr|Phi|Pi|PlusMinus|Poincareplane|Popf|Pr|Precedes|PrecedesEqual|PrecedesSlantEqual|PrecedesTilde|Prime|Product|Proportion|Proportional|Pscr|Psi|QUO|QUOT|Qfr|Qopf|Qscr|RBarr|RE|REG|Racute|Rang|Rarr|Rarrtl|Rcaron|Rcedil|Rcy|Re|ReverseElement|ReverseEquilibrium|ReverseUpEquilibrium|Rfr|Rho|RightAngleBracket|RightArrow|RightArrowBar|RightArrowLeftArrow|RightCeiling|RightDoubleBracket|RightDownTeeVector|RightDownVector|RightDownVectorBar|RightFloor|RightTee|RightTeeArrow|RightTeeVector|RightTriangle|RightTriangleBar|RightTriangleEqual|RightUpDownVector|RightUpTeeVector|RightUpVector|RightUpVectorBar|RightVector|RightVectorBar|Rightarrow|Ropf|RoundImplies|Rrightarrow|Rscr|Rsh|RuleDelayed|SHCHcy|SHcy|SOFTcy|Sacute|Sc|Scaron|Scedil|Scirc|Scy|Sfr|ShortDownArrow|ShortLeftArrow|ShortRightArrow|ShortUpArrow|Sigma|SmallCircle|Sopf|Sqrt|Square|SquareIntersection|SquareSubset|SquareSubsetEqual|SquareSuperset|SquareSupersetEqual|SquareUnion|Sscr|Star|Sub|Subset|SubsetEqual|Succeeds|SucceedsEqual|SucceedsSlantEqual|SucceedsTilde|SuchThat|Sum|Sup|Superset|SupersetEqual|Supset|THOR|THORN|TRADE|TSHcy|TScy|Tab|Tau|Tcaron|Tcedil|Tcy|Tfr|Therefore|Theta|ThickSpace|ThinSpace|Tilde|TildeEqual|TildeFullEqual|TildeTilde|Topf|TripleDot|Tscr|Tstrok|Uacut|Uacute|Uarr|Uarrocir|Ubrcy|Ubreve|Ucir|Ucirc|Ucy|Udblac|Ufr|Ugrav|Ugrave|Umacr|UnderBar|UnderBrace|UnderBracket|UnderParenthesis|Union|UnionPlus|Uogon|Uopf|UpArrow|UpArrowBar|UpArrowDownArrow|UpDownArrow|UpEquilibrium|UpTee|UpTeeArrow|Uparrow|Updownarrow|UpperLeftArrow|UpperRightArrow|Upsi|Upsilon|Uring|Uscr|Utilde|Uum|Uuml|VDash|Vbar|Vcy|Vdash|Vdashl|Vee|Verbar|Vert|VerticalBar|VerticalLine|VerticalSeparator|VerticalTilde|VeryThinSpace|Vfr|Vopf|Vscr|Vvdash|Wcirc|Wedge|Wfr|Wopf|Wscr|Xfr|Xi|Xopf|Xscr|YAcy|YIcy|YUcy|Yacut|Yacute|Ycirc|Ycy|Yfr|Yopf|Yscr|Yuml|ZHcy|Zacute|Zcaron|Zcy|Zdot|ZeroWidthSpace|Zeta|Zfr|Zopf|Zscr|aacut|aacute|abreve|ac|acE|acd|acir|acirc|acut|acute|acy|aeli|aelig|af|afr|agrav|agrave|alefsym|aleph|alpha|amacr|amalg|am|amp|and|andand|andd|andslope|andv|ang|ange|angle|angmsd|angmsdaa|angmsdab|angmsdac|angmsdad|angmsdae|angmsdaf|angmsdag|angmsdah|angrt|angrtvb|angrtvbd|angsph|angst|angzarr|aogon|aopf|ap|apE|apacir|ape|apid|apos|approx|approxeq|arin|aring|ascr|ast|asymp|asympeq|atild|atilde|aum|auml|awconint|awint|bNot|backcong|backepsilon|backprime|backsim|backsimeq|barvee|barwed|barwedge|bbrk|bbrktbrk|bcong|bcy|bdquo|becaus|because|bemptyv|bepsi|bernou|beta|beth|between|bfr|bigcap|bigcirc|bigcup|bigodot|bigoplus|bigotimes|bigsqcup|bigstar|bigtriangledown|bigtriangleup|biguplus|bigvee|bigwedge|bkarow|blacklozenge|blacksquare|blacktriangle|blacktriangledown|blacktriangleleft|blacktriangleright|blank|blk12|blk14|blk34|block|bne|bnequiv|bnot|bopf|bot|bottom|bowtie|boxDL|boxDR|boxDl|boxDr|boxH|boxHD|boxHU|boxHd|boxHu|boxUL|boxUR|boxUl|boxUr|boxV|boxVH|boxVL|boxVR|boxVh|boxVl|boxVr|boxbox|boxdL|boxdR|boxdl|boxdr|boxh|boxhD|boxhU|boxhd|boxhu|boxminus|boxplus|boxtimes|boxuL|boxuR|boxul|boxur|boxv|boxvH|boxvL|boxvR|boxvh|boxvl|boxvr|bprime|breve|brvba|brvbar|bscr|bsemi|bsim|bsime|bsol|bsolb|bsolhsub|bull|bullet|bump|bumpE|bumpe|bumpeq|cacute|cap|capand|capbrcup|capcap|capcup|capdot|caps|caret|caron|ccaps|ccaron|ccedi|ccedil|ccirc|ccups|ccupssm|cdot|cedi|cedil|cemptyv|cen|cent|centerdot|cfr|chcy|check|checkmark|chi|cir|cirE|circ|circeq|circlearrowleft|circlearrowright|circledR|circledS|circledast|circledcirc|circleddash|cire|cirfnint|cirmid|cirscir|clubs|clubsuit|colon|colone|coloneq|comma|commat|comp|compfn|complement|complexes|cong|congdot|conint|copf|coprod|cop|copy|copysr|crarr|cross|cscr|csub|csube|csup|csupe|ctdot|cudarrl|cudarrr|cuepr|cuesc|cularr|cularrp|cup|cupbrcap|cupcap|cupcup|cupdot|cupor|cups|curarr|curarrm|curlyeqprec|curlyeqsucc|curlyvee|curlywedge|curre|curren|curvearrowleft|curvearrowright|cuvee|cuwed|cwconint|cwint|cylcty|dArr|dHar|dagger|daleth|darr|dash|dashv|dbkarow|dblac|dcaron|dcy|dd|ddagger|ddarr|ddotseq|de|deg|delta|demptyv|dfisht|dfr|dharl|dharr|diam|diamond|diamondsuit|diams|die|digamma|disin|div|divid|divide|divideontimes|divonx|djcy|dlcorn|dlcrop|dollar|dopf|dot|doteq|doteqdot|dotminus|dotplus|dotsquare|doublebarwedge|downarrow|downdownarrows|downharpoonleft|downharpoonright|drbkarow|drcorn|drcrop|dscr|dscy|dsol|dstrok|dtdot|dtri|dtrif|duarr|duhar|dwangle|dzcy|dzigrarr|eDDot|eDot|eacut|eacute|easter|ecaron|ecir|ecir|ecirc|ecolon|ecy|edot|ee|efDot|efr|eg|egrav|egrave|egs|egsdot|el|elinters|ell|els|elsdot|emacr|empty|emptyset|emptyv|emsp13|emsp14|emsp|eng|ensp|eogon|eopf|epar|eparsl|eplus|epsi|epsilon|epsiv|eqcirc|eqcolon|eqsim|eqslantgtr|eqslantless|equals|equest|equiv|equivDD|eqvparsl|erDot|erarr|escr|esdot|esim|eta|et|eth|eum|euml|euro|excl|exist|expectation|exponentiale|fallingdotseq|fcy|female|ffilig|fflig|ffllig|ffr|filig|fjlig|flat|fllig|fltns|fnof|fopf|forall|fork|forkv|fpartint|frac1|frac12|frac13|frac1|frac14|frac15|frac16|frac18|frac23|frac25|frac3|frac34|frac35|frac38|frac45|frac56|frac58|frac78|frasl|frown|fscr|gE|gEl|gacute|gamma|gammad|gap|gbreve|gcirc|gcy|gdot|ge|gel|geq|geqq|geqslant|ges|gescc|gesdot|gesdoto|gesdotol|gesl|gesles|gfr|gg|ggg|gimel|gjcy|gl|glE|gla|glj|gnE|gnap|gnapprox|gne|gneq|gneqq|gnsim|gopf|grave|gscr|gsim|gsime|gsiml|g|gt|gtcc|gtcir|gtdot|gtlPar|gtquest|gtrapprox|gtrarr|gtrdot|gtreqless|gtreqqless|gtrless|gtrsim|gvertneqq|gvnE|hArr|hairsp|half|hamilt|hardcy|harr|harrcir|harrw|hbar|hcirc|hearts|heartsuit|hellip|hercon|hfr|hksearow|hkswarow|hoarr|homtht|hookleftarrow|hookrightarrow|hopf|horbar|hscr|hslash|hstrok|hybull|hyphen|iacut|iacute|ic|icir|icirc|icy|iecy|iexc|iexcl|iff|ifr|igrav|igrave|ii|iiiint|iiint|iinfin|iiota|ijlig|imacr|image|imagline|imagpart|imath|imof|imped|in|incare|infin|infintie|inodot|int|intcal|integers|intercal|intlarhk|intprod|iocy|iogon|iopf|iota|iprod|iques|iquest|iscr|isin|isinE|isindot|isins|isinsv|isinv|it|itilde|iukcy|ium|iuml|jcirc|jcy|jfr|jmath|jopf|jscr|jsercy|jukcy|kappa|kappav|kcedil|kcy|kfr|kgreen|khcy|kjcy|kopf|kscr|lAarr|lArr|lAtail|lBarr|lE|lEg|lHar|lacute|laemptyv|lagran|lambda|lang|langd|langle|lap|laqu|laquo|larr|larrb|larrbfs|larrfs|larrhk|larrlp|larrpl|larrsim|larrtl|lat|latail|late|lates|lbarr|lbbrk|lbrace|lbrack|lbrke|lbrksld|lbrkslu|lcaron|lcedil|lceil|lcub|lcy|ldca|ldquo|ldquor|ldrdhar|ldrushar|ldsh|le|leftarrow|leftarrowtail|leftharpoondown|leftharpoonup|leftleftarrows|leftrightarrow|leftrightarrows|leftrightharpoons|leftrightsquigarrow|leftthreetimes|leg|leq|leqq|leqslant|les|lescc|lesdot|lesdoto|lesdotor|lesg|lesges|lessapprox|lessdot|lesseqgtr|lesseqqgtr|lessgtr|lesssim|lfisht|lfloor|lfr|lg|lgE|lhard|lharu|lharul|lhblk|ljcy|ll|llarr|llcorner|llhard|lltri|lmidot|lmoust|lmoustache|lnE|lnap|lnapprox|lne|lneq|lneqq|lnsim|loang|loarr|lobrk|longleftarrow|longleftrightarrow|longmapsto|longrightarrow|looparrowleft|looparrowright|lopar|lopf|loplus|lotimes|lowast|lowbar|loz|lozenge|lozf|lpar|lparlt|lrarr|lrcorner|lrhar|lrhard|lrm|lrtri|lsaquo|lscr|lsh|lsim|lsime|lsimg|lsqb|lsquo|lsquor|lstrok|l|lt|ltcc|ltcir|ltdot|lthree|ltimes|ltlarr|ltquest|ltrPar|ltri|ltrie|ltrif|lurdshar|luruhar|lvertneqq|lvnE|mDDot|mac|macr|male|malt|maltese|map|mapsto|mapstodown|mapstoleft|mapstoup|marker|mcomma|mcy|mdash|measuredangle|mfr|mho|micr|micro|mid|midast|midcir|middo|middot|minus|minusb|minusd|minusdu|mlcp|mldr|mnplus|models|mopf|mp|mscr|mstpos|mu|multimap|mumap|nGg|nGt|nGtv|nLeftarrow|nLeftrightarrow|nLl|nLt|nLtv|nRightarrow|nVDash|nVdash|nabla|nacute|nang|nap|napE|napid|napos|napprox|natur|natural|naturals|nbs|nbsp|nbump|nbumpe|ncap|ncaron|ncedil|ncong|ncongdot|ncup|ncy|ndash|ne|neArr|nearhk|nearr|nearrow|nedot|nequiv|nesear|nesim|nexist|nexists|nfr|ngE|nge|ngeq|ngeqq|ngeqslant|nges|ngsim|ngt|ngtr|nhArr|nharr|nhpar|ni|nis|nisd|niv|njcy|nlArr|nlE|nlarr|nldr|nle|nleftarrow|nleftrightarrow|nleq|nleqq|nleqslant|nles|nless|nlsim|nlt|nltri|nltrie|nmid|nopf|no|not|notin|notinE|notindot|notinva|notinvb|notinvc|notni|notniva|notnivb|notnivc|npar|nparallel|nparsl|npart|npolint|npr|nprcue|npre|nprec|npreceq|nrArr|nrarr|nrarrc|nrarrw|nrightarrow|nrtri|nrtrie|nsc|nsccue|nsce|nscr|nshortmid|nshortparallel|nsim|nsime|nsimeq|nsmid|nspar|nsqsube|nsqsupe|nsub|nsubE|nsube|nsubset|nsubseteq|nsubseteqq|nsucc|nsucceq|nsup|nsupE|nsupe|nsupset|nsupseteq|nsupseteqq|ntgl|ntild|ntilde|ntlg|ntriangleleft|ntrianglelefteq|ntriangleright|ntrianglerighteq|nu|num|numero|numsp|nvDash|nvHarr|nvap|nvdash|nvge|nvgt|nvinfin|nvlArr|nvle|nvlt|nvltrie|nvrArr|nvrtrie|nvsim|nwArr|nwarhk|nwarr|nwarrow|nwnear|oS|oacut|oacute|oast|ocir|ocir|ocirc|ocy|odash|odblac|odiv|odot|odsold|oelig|ofcir|ofr|ogon|ograv|ograve|ogt|ohbar|ohm|oint|olarr|olcir|olcross|oline|olt|omacr|omega|omicron|omid|ominus|oopf|opar|operp|oplus|or|orarr|ord|order|orderof|ord|ordf|ord|ordm|origof|oror|orslope|orv|oscr|oslas|oslash|osol|otild|otilde|otimes|otimesas|oum|ouml|ovbar|par|par|para|parallel|parsim|parsl|part|pcy|percnt|period|permil|perp|pertenk|pfr|phi|phiv|phmmat|phone|pi|pitchfork|piv|planck|planckh|plankv|plus|plusacir|plusb|pluscir|plusdo|plusdu|pluse|plusm|plusmn|plussim|plustwo|pm|pointint|popf|poun|pound|pr|prE|prap|prcue|pre|prec|precapprox|preccurlyeq|preceq|precnapprox|precneqq|precnsim|precsim|prime|primes|prnE|prnap|prnsim|prod|profalar|profline|profsurf|prop|propto|prsim|prurel|pscr|psi|puncsp|qfr|qint|qopf|qprime|qscr|quaternions|quatint|quest|questeq|quo|quot|rAarr|rArr|rAtail|rBarr|rHar|race|racute|radic|raemptyv|rang|rangd|range|rangle|raqu|raquo|rarr|rarrap|rarrb|rarrbfs|rarrc|rarrfs|rarrhk|rarrlp|rarrpl|rarrsim|rarrtl|rarrw|ratail|ratio|rationals|rbarr|rbbrk|rbrace|rbrack|rbrke|rbrksld|rbrkslu|rcaron|rcedil|rceil|rcub|rcy|rdca|rdldhar|rdquo|rdquor|rdsh|real|realine|realpart|reals|rect|re|reg|rfisht|rfloor|rfr|rhard|rharu|rharul|rho|rhov|rightarrow|rightarrowtail|rightharpoondown|rightharpoonup|rightleftarrows|rightleftharpoons|rightrightarrows|rightsquigarrow|rightthreetimes|ring|risingdotseq|rlarr|rlhar|rlm|rmoust|rmoustache|rnmid|roang|roarr|robrk|ropar|ropf|roplus|rotimes|rpar|rpargt|rppolint|rrarr|rsaquo|rscr|rsh|rsqb|rsquo|rsquor|rthree|rtimes|rtri|rtrie|rtrif|rtriltri|ruluhar|rx|sacute|sbquo|sc|scE|scap|scaron|sccue|sce|scedil|scirc|scnE|scnap|scnsim|scpolint|scsim|scy|sdot|sdotb|sdote|seArr|searhk|searr|searrow|sec|sect|semi|seswar|setminus|setmn|sext|sfr|sfrown|sharp|shchcy|shcy|shortmid|shortparallel|sh|shy|sigma|sigmaf|sigmav|sim|simdot|sime|simeq|simg|simgE|siml|simlE|simne|simplus|simrarr|slarr|smallsetminus|smashp|smeparsl|smid|smile|smt|smte|smtes|softcy|sol|solb|solbar|sopf|spades|spadesuit|spar|sqcap|sqcaps|sqcup|sqcups|sqsub|sqsube|sqsubset|sqsubseteq|sqsup|sqsupe|sqsupset|sqsupseteq|squ|square|squarf|squf|srarr|sscr|ssetmn|ssmile|sstarf|star|starf|straightepsilon|straightphi|strns|sub|subE|subdot|sube|subedot|submult|subnE|subne|subplus|subrarr|subset|subseteq|subseteqq|subsetneq|subsetneqq|subsim|subsub|subsup|succ|succapprox|succcurlyeq|succeq|succnapprox|succneqq|succnsim|succsim|sum|sung|sup|sup1|sup|sup2|sup|sup3|sup|supE|supdot|supdsub|supe|supedot|suphsol|suphsub|suplarr|supmult|supnE|supne|supplus|supset|supseteq|supseteqq|supsetneq|supsetneqq|supsim|supsub|supsup|swArr|swarhk|swarr|swarrow|swnwar|szli|szlig|target|tau|tbrk|tcaron|tcedil|tcy|tdot|telrec|tfr|there4|therefore|theta|thetasym|thetav|thickapprox|thicksim|thinsp|thkap|thksim|thor|thorn|tilde|time|times|timesb|timesbar|timesd|tint|toea|top|topbot|topcir|topf|topfork|tosa|tprime|trade|triangle|triangledown|triangleleft|trianglelefteq|triangleq|triangleright|trianglerighteq|tridot|trie|triminus|triplus|trisb|tritime|trpezium|tscr|tscy|tshcy|tstrok|twixt|twoheadleftarrow|twoheadrightarrow|uArr|uHar|uacut|uacute|uarr|ubrcy|ubreve|ucir|ucirc|ucy|udarr|udblac|udhar|ufisht|ufr|ugrav|ugrave|uharl|uharr|uhblk|ulcorn|ulcorner|ulcrop|ultri|umacr|um|uml|uogon|uopf|uparrow|updownarrow|upharpoonleft|upharpoonright|uplus|upsi|upsih|upsilon|upuparrows|urcorn|urcorner|urcrop|uring|urtri|uscr|utdot|utilde|utri|utrif|uuarr|uum|uuml|uwangle|vArr|vBar|vBarv|vDash|vangrt|varepsilon|varkappa|varnothing|varphi|varpi|varpropto|varr|varrho|varsigma|varsubsetneq|varsubsetneqq|varsupsetneq|varsupsetneqq|vartheta|vartriangleleft|vartriangleright|vcy|vdash|vee|veebar|veeeq|vellip|verbar|vert|vfr|vltri|vnsub|vnsup|vopf|vprop|vrtri|vscr|vsubnE|vsubne|vsupnE|vsupne|vzigzag|wcirc|wedbar|wedge|wedgeq|weierp|wfr|wopf|wp|wr|wreath|wscr|xcap|xcirc|xcup|xdtri|xfr|xhArr|xharr|xi|xlArr|xlarr|xmap|xnis|xodot|xopf|xoplus|xotime|xrArr|xrarr|xscr|xsqcup|xuplus|xutri|xvee|xwedge|yacut|yacute|yacy|ycirc|ycy|ye|yen|yfr|yicy|yopf|yscr|yucy|yum|yuml|zacute|zcaron|zcy|zdot|zeetrf|zeta|zfr|zhcy|zigrarr|zopf|zscr|zwj|zwnj);`))

	// HTML numeric character reference like &#123; or &#x1F;
	g.Define("numeric_character_reference",
		Pat(`&#([0-9]{1,7}|[xX][0-9a-fA-F]{1,6});`))

	// square-bracket-delimited link label used for reference links
	g.Define("link_label",
		Seq(
			Str("["),
			Repeat1(Choice(
				Sym("_text_inline_no_link"),
				Sym("backslash_escape"),
				Sym("entity_reference"),
				Sym("numeric_character_reference"),
				Sym("_soft_line_break"))),
			Str("]")))

	// URL or path for a link, either angle-bracket or bare form
	g.Define("link_destination",
		PrecDynamic(10, Choice(
			Seq(
				Str("<"),
				Repeat(Choice(
					Sym("_text_no_angle"),
					Sym("backslash_escape"),
					Sym("entity_reference"),
					Sym("numeric_character_reference"))),
				Str(">")),
			Seq(
				Choice(
					Sym("_word"),
					Seq(
						Choice(
							Str("!"),
							Str("\""),
							Str("#"),
							Str("$"),
							Str("%"),
							Str("&"),
							Str("'"),
							Str("*"),
							Str("+"),
							Str(","),
							Str("-"),
							Str("."),
							Str("/"),
							Str(":"),
							Str(";"),
							Str("="),
							Str(">"),
							Str("?"),
							Str("@"),
							Str("["),
							Str("\\"),
							Str("]"),
							Str("^"),
							Str("_"),
							Str("`"),
							Str("{"),
							Str("|"),
							Str("}"),
							Str("~")),
						Choice(
							Sym("_last_token_punctuation"),
							Blank())),
					Sym("backslash_escape"),
					Sym("entity_reference"),
					Sym("numeric_character_reference"),
					Sym("_link_destination_parenthesis")),
				Repeat(Choice(
					Sym("_word"),
					Seq(
						Choice(
							Str("!"),
							Str("\""),
							Str("#"),
							Str("$"),
							Str("%"),
							Str("&"),
							Str("'"),
							Str("*"),
							Str("+"),
							Str(","),
							Str("-"),
							Str("."),
							Str("/"),
							Str(":"),
							Str(";"),
							Str("<"),
							Str("="),
							Str(">"),
							Str("?"),
							Str("@"),
							Str("["),
							Str("\\"),
							Str("]"),
							Str("^"),
							Str("_"),
							Str("`"),
							Str("{"),
							Str("|"),
							Str("}"),
							Str("~")),
						Choice(
							Sym("_last_token_punctuation"),
							Blank())),
					Sym("backslash_escape"),
					Sym("entity_reference"),
					Sym("numeric_character_reference"),
					Sym("_link_destination_parenthesis")))))))

	// balanced parentheses inside a link destination
	g.Define("_link_destination_parenthesis",
		Seq(
			Str("("),
			Repeat(Choice(
				Sym("_word"),
				Seq(
					Choice(
						Str("!"),
						Str("\""),
						Str("#"),
						Str("$"),
						Str("%"),
						Str("&"),
						Str("'"),
						Str("*"),
						Str("+"),
						Str(","),
						Str("-"),
						Str("."),
						Str("/"),
						Str(":"),
						Str(";"),
						Str("<"),
						Str("="),
						Str(">"),
						Str("?"),
						Str("@"),
						Str("["),
						Str("\\"),
						Str("]"),
						Str("^"),
						Str("_"),
						Str("`"),
						Str("{"),
						Str("|"),
						Str("}"),
						Str("~")),
					Choice(
						Sym("_last_token_punctuation"),
						Blank())),
				Sym("backslash_escape"),
				Sym("entity_reference"),
				Sym("numeric_character_reference"),
				Sym("_link_destination_parenthesis"))),
			Str(")")))

	// any text characters except angle brackets (used in link destinations)
	g.Define("_text_no_angle",
		Choice(
			Sym("_word"),
			Seq(
				Choice(
					Str("!"),
					Str("\""),
					Str("#"),
					Str("$"),
					Str("%"),
					Str("&"),
					Str("'"),
					Str("("),
					Str(")"),
					Str("*"),
					Str("+"),
					Str(","),
					Str("-"),
					Str("."),
					Str("/"),
					Str(":"),
					Str(";"),
					Str("="),
					Str("?"),
					Str("@"),
					Str("["),
					Str("\\"),
					Str("]"),
					Str("^"),
					Str("_"),
					Str("`"),
					Str("{"),
					Str("|"),
					Str("}"),
					Str("~")),
				Choice(
					Sym("_last_token_punctuation"),
					Blank())),
			Sym("_whitespace")))

	// optional title attribute for a link, in quotes or parens
	g.Define("link_title",
		Choice(
			Seq(
				Str("\""),
				Repeat(Choice(
					Sym("_word"),
					Seq(
						Choice(
							Str("!"),
							Str("#"),
							Str("$"),
							Str("%"),
							Str("&"),
							Str("'"),
							Str("("),
							Str(")"),
							Str("*"),
							Str("+"),
							Str(","),
							Str("-"),
							Str("."),
							Str("/"),
							Str(":"),
							Str(";"),
							Str("<"),
							Str("="),
							Str(">"),
							Str("?"),
							Str("@"),
							Str("["),
							Str("\\"),
							Str("]"),
							Str("^"),
							Str("_"),
							Str("`"),
							Str("{"),
							Str("|"),
							Str("}"),
							Str("~")),
						Choice(
							Sym("_last_token_punctuation"),
							Blank())),
					Sym("_whitespace"),
					Sym("backslash_escape"),
					Sym("entity_reference"),
					Sym("numeric_character_reference"),
					Seq(
						Sym("_soft_line_break"),
						Choice(
							Seq(
								Sym("_soft_line_break"),
								Sym("_trigger_error")),
							Blank())))),
				Str("\"")),
			Seq(
				Str("'"),
				Repeat(Choice(
					Sym("_word"),
					Seq(
						Choice(
							Str("!"),
							Str("\""),
							Str("#"),
							Str("$"),
							Str("%"),
							Str("&"),
							Str("("),
							Str(")"),
							Str("*"),
							Str("+"),
							Str(","),
							Str("-"),
							Str("."),
							Str("/"),
							Str(":"),
							Str(";"),
							Str("<"),
							Str("="),
							Str(">"),
							Str("?"),
							Str("@"),
							Str("["),
							Str("\\"),
							Str("]"),
							Str("^"),
							Str("_"),
							Str("`"),
							Str("{"),
							Str("|"),
							Str("}"),
							Str("~")),
						Choice(
							Sym("_last_token_punctuation"),
							Blank())),
					Sym("_whitespace"),
					Sym("backslash_escape"),
					Sym("entity_reference"),
					Sym("numeric_character_reference"),
					Seq(
						Sym("_soft_line_break"),
						Choice(
							Seq(
								Sym("_soft_line_break"),
								Sym("_trigger_error")),
							Blank())))),
				Str("'")),
			Seq(
				Str("("),
				Repeat(Choice(
					Sym("_word"),
					Seq(
						Choice(
							Str("!"),
							Str("\""),
							Str("#"),
							Str("$"),
							Str("%"),
							Str("&"),
							Str("'"),
							Str("*"),
							Str("+"),
							Str(","),
							Str("-"),
							Str("."),
							Str("/"),
							Str(":"),
							Str(";"),
							Str("<"),
							Str("="),
							Str(">"),
							Str("?"),
							Str("@"),
							Str("["),
							Str("\\"),
							Str("]"),
							Str("^"),
							Str("_"),
							Str("`"),
							Str("{"),
							Str("|"),
							Str("}"),
							Str("~")),
						Choice(
							Sym("_last_token_punctuation"),
							Blank())),
					Sym("_whitespace"),
					Sym("backslash_escape"),
					Sym("entity_reference"),
					Sym("numeric_character_reference"),
					Seq(
						Sym("_soft_line_break"),
						Choice(
							Seq(
								Sym("_soft_line_break"),
								Sym("_trigger_error")),
							Blank())))),
				Str(")"))))

	// external line-ending token (blocks)
	g.Define("_newline_token",
		Pat(`\n|\r\n?`))

	// prevents trailing punctuation in link destinations
	g.Define("_last_token_punctuation",
		Choice(
			))

	// any block-level element (choice dispatch)
	g.Define("_block",
		Choice(
			Sym("_block_not_section"),
			Sym("section")))

	// any block element that is not a section heading
	g.Define("_block_not_section",
		Choice(
			Alias(Sym("_setext_heading1"), "setext_heading", true),
			Alias(Sym("_setext_heading2"), "setext_heading", true),
			Sym("paragraph"),
			Sym("indented_code_block"),
			Sym("_block_quote"),
			Sym("thematic_break"),
			Sym("_list"),
			Alias(Sym("_fenced_code_block"), "fenced_code_block", true),
			Sym("_blank_line"),
			Sym("html_block"),
			Sym("link_reference_definition"),
			Sym("_pipe_table")))

	// a document section introduced by an ATX heading
	g.Define("section",
		Choice(
			Sym("_section1"),
			Sym("_section2"),
			Sym("_section3"),
			Sym("_section4"),
			Sym("_section5"),
			Sym("_section6")))

	// content of a level-1 section
	g.Define("_section1",
		PrecRight(0, Seq(
			Alias(Sym("_atx_heading1"), "atx_heading", true),
			Repeat(Choice(
				Alias(Choice(
					Sym("_section6"),
					Sym("_section5"),
					Sym("_section4"),
					Sym("_section3"),
					Sym("_section2")), "section", true),
				Sym("_block_not_section"))))))

	// content of a level-2 section
	g.Define("_section2",
		PrecRight(0, Seq(
			Alias(Sym("_atx_heading2"), "atx_heading", true),
			Repeat(Choice(
				Alias(Choice(
					Sym("_section6"),
					Sym("_section5"),
					Sym("_section4"),
					Sym("_section3")), "section", true),
				Sym("_block_not_section"))))))

	// content of a level-3 section
	g.Define("_section3",
		PrecRight(0, Seq(
			Alias(Sym("_atx_heading3"), "atx_heading", true),
			Repeat(Choice(
				Alias(Choice(
					Sym("_section6"),
					Sym("_section5"),
					Sym("_section4")), "section", true),
				Sym("_block_not_section"))))))

	// content of a level-4 section
	g.Define("_section4",
		PrecRight(0, Seq(
			Alias(Sym("_atx_heading4"), "atx_heading", true),
			Repeat(Choice(
				Alias(Choice(
					Sym("_section6"),
					Sym("_section5")), "section", true),
				Sym("_block_not_section"))))))

	// content of a level-5 section
	g.Define("_section5",
		PrecRight(0, Seq(
			Alias(Sym("_atx_heading5"), "atx_heading", true),
			Repeat(Choice(
				Alias(Sym("_section6"), "section", true),
				Sym("_block_not_section"))))))

	// content of a level-6 section
	g.Define("_section6",
		PrecRight(0, Seq(
			Alias(Sym("_atx_heading6"), "atx_heading", true),
			Repeat(Sym("_block_not_section")))))

	// horizontal rule (---, ***, or ___)
	g.Define("thematic_break",
		Seq(
			Sym("_thematic_break"),
			Choice(
				Sym("_newline"),
				Sym("_eof"))))

	// ATX-style level-1 heading (# ...)
	g.Define("_atx_heading1",
		Prec(1, Seq(
			Sym("atx_h1_marker"),
			Choice(
				Sym("_atx_heading_content"),
				Blank()),
			Sym("_newline"))))

	// ATX-style level-2 heading (## ...)
	g.Define("_atx_heading2",
		Prec(1, Seq(
			Sym("atx_h2_marker"),
			Choice(
				Sym("_atx_heading_content"),
				Blank()),
			Sym("_newline"))))

	// ATX-style level-3 heading (### ...)
	g.Define("_atx_heading3",
		Prec(1, Seq(
			Sym("atx_h3_marker"),
			Choice(
				Sym("_atx_heading_content"),
				Blank()),
			Sym("_newline"))))

	// ATX-style level-4 heading (#### ...)
	g.Define("_atx_heading4",
		Prec(1, Seq(
			Sym("atx_h4_marker"),
			Choice(
				Sym("_atx_heading_content"),
				Blank()),
			Sym("_newline"))))

	// ATX-style level-5 heading (##### ...)
	g.Define("_atx_heading5",
		Prec(1, Seq(
			Sym("atx_h5_marker"),
			Choice(
				Sym("_atx_heading_content"),
				Blank()),
			Sym("_newline"))))

	// ATX-style level-6 heading (###### ...)
	g.Define("_atx_heading6",
		Prec(1, Seq(
			Sym("atx_h6_marker"),
			Choice(
				Sym("_atx_heading_content"),
				Blank()),
			Sym("_newline"))))

	// inline content inside an ATX heading
	g.Define("_atx_heading_content",
		Prec(1, Seq(
			Choice(
				Sym("_whitespace"),
				Blank()),
			Field("heading_content", Alias(Sym("_line"), "inline", true)))))

	// setext-style level-1 heading (text + === underline)
	g.Define("_setext_heading1",
		Seq(
			Field("heading_content", Sym("paragraph")),
			Sym("setext_h1_underline"),
			Choice(
				Sym("_newline"),
				Sym("_eof"))))

	// setext-style level-2 heading (text + --- underline)
	g.Define("_setext_heading2",
		Seq(
			Field("heading_content", Sym("paragraph")),
			Sym("setext_h2_underline"),
			Choice(
				Sym("_newline"),
				Sym("_eof"))))

	// code block created by 4-space indentation
	g.Define("indented_code_block",
		PrecRight(0, Seq(
			Sym("_indented_chunk"),
			Repeat(Choice(
				Sym("_indented_chunk"),
				Sym("_blank_line"))))))

	// a single run of indented lines inside an indented code block
	g.Define("_indented_chunk",
		Seq(
			Sym("_indented_chunk_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_indented_chunk_newline"))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// fenced code block delimited by ``` or ~~~
	// Hidden by name (underscore prefix) — children flatten into parent rule
	// at the top level. The bundled markdown.bin parser emits zero
	// fenced_code_block wrapper nodes at top level (the delimiters / info /
	// content appear as flat siblings under `section`), but DOES emit a
	// visible `fenced_code_block` wrapper when the fence is inside a container
	// (list_item, block_quote). The rule stays hidden at top level via the
	// underscore prefix, and is re-aliased to visible `fenced_code_block`
	// inside `_block_in_container` to match the bundled CST shape.
	g.Define("_fenced_code_block",
		PrecRight(0, Choice(
			Seq(
				Alias(Sym("_fenced_code_block_start_backtick"), "fenced_code_block_delimiter", true),
				Choice(
					Sym("_whitespace"),
					Blank()),
				Choice(
					Sym("info_string"),
					Blank()),
				Sym("_fenced_code_block_newline"),
				Choice(
					Sym("code_fence_content"),
					Blank()),
				Choice(
					Seq(
						Alias(Sym("_fenced_code_block_end_backtick"), "fenced_code_block_delimiter", true),
						Sym("_close_block"),
						Sym("_fenced_code_block_newline")),
					Blank()),
				Sym("_block_close")),
			Seq(
				Alias(Sym("_fenced_code_block_start_tilde"), "fenced_code_block_delimiter", true),
				Choice(
					Sym("_whitespace"),
					Blank()),
				Choice(
					Sym("info_string"),
					Blank()),
				Sym("_fenced_code_block_newline"),
				Choice(
					Sym("code_fence_content"),
					Blank()),
				Choice(
					Seq(
						Alias(Sym("_fenced_code_block_end_tilde"), "fenced_code_block_delimiter", true),
						Sym("_close_block"),
						Sym("_fenced_code_block_newline")),
					Blank()),
				Sym("_block_close")))))

	// raw text content between fenced code block delimiters.
	// `_fenced_code_block_line` is a structurally-identical copy of `_line`
	// used only here; the distinct rule name keeps `code_fence_content`'s
	// content states from merging with paragraph `_line` states. Without the
	// split, fences after non-paragraph blocks (heading, blank-line, etc.)
	// would re-enter the merged content state where `_close_block` is in the
	// reduce-lookahead set, and the scanner's eager `_close_block` branch
	// (markdown_scanner.go line 344) fires before the body is parsed.
	g.Define("code_fence_content",
		Repeat1(Choice(
			Sym("_fenced_code_block_newline"),
			Sym("_fenced_code_block_line"))))

	// language/info tag on the opening fence line
	g.Define("info_string",
		Choice(
			Seq(
				Sym("language"),
				Repeat(Choice(
					Sym("_line"),
					Sym("backslash_escape"),
					Sym("entity_reference"),
					Sym("numeric_character_reference")))),
			Seq(
				Repeat1(Choice(
					Str("{"),
					Str("}"))),
				Choice(
					Choice(
						Seq(
							Sym("language"),
							Repeat(Choice(
								Sym("_line"),
								Sym("backslash_escape"),
								Sym("entity_reference"),
								Sym("numeric_character_reference")))),
						Seq(
							Sym("_whitespace"),
							Repeat(Choice(
								Sym("_line"),
								Sym("backslash_escape"),
								Sym("entity_reference"),
								Sym("numeric_character_reference"))))),
					Blank()))))

	// the language identifier inside an info string
	g.Define("language",
		PrecRight(0, Repeat1(Choice(
			Sym("_word"),
			Seq(
				Choice(
					Str("!"),
					Str("\""),
					Str("#"),
					Str("$"),
					Str("%"),
					Str("&"),
					Str("'"),
					Str("("),
					Str(")"),
					Str("*"),
					Str("+"),
					Str("-"),
					Str("."),
					Str("/"),
					Str(":"),
					Str(";"),
					Str("<"),
					Str("="),
					Str(">"),
					Str("?"),
					Str("@"),
					Str("["),
					Str("\\"),
					Str("]"),
					Str("^"),
					Str("_"),
					Str("`"),
					Str("|"),
					Str("~")),
				Choice(
					Sym("_last_token_punctuation"),
					Blank())),
			Sym("backslash_escape"),
			Sym("entity_reference"),
			Sym("numeric_character_reference")))))

	// raw HTML block (one of the 7 HTML block types)
	g.Define("html_block",
		Prec(1, Seq(
			Choice(
				Sym("_whitespace"),
				Blank()),
			Choice(
				Sym("_html_block_1"),
				Sym("_html_block_2"),
				Sym("_html_block_3"),
				Sym("_html_block_4"),
				Sym("_html_block_5"),
				Sym("_html_block_6"),
				Sym("_html_block_7")))))

	// HTML block type 1: <pre>/<script>/<style>/<textarea> until end tag
	g.Define("_html_block_1",
		Seq(
			Sym("_html_block_1_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_html_block_newline"),
				Seq(
					Sym("_html_block_1_end"),
					Sym("_close_block")))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// HTML block type 2: <!-- ... -->
	g.Define("_html_block_2",
		Seq(
			Sym("_html_block_2_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_html_block_newline"),
				Seq(
					Str("-->"),
					Sym("_close_block")))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// HTML block type 3: <? ... ?>
	g.Define("_html_block_3",
		Seq(
			Sym("_html_block_3_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_html_block_newline"),
				Seq(
					Str("?>"),
					Sym("_close_block")))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// HTML block type 4: <!LETTER ... >
	g.Define("_html_block_4",
		Seq(
			Sym("_html_block_4_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_html_block_newline"),
				Seq(
					Str(">"),
					Sym("_close_block")))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// HTML block type 5: <![CDATA[ ... ]]>
	g.Define("_html_block_5",
		Seq(
			Sym("_html_block_5_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_html_block_newline"),
				Seq(
					Str("]]>"),
					Sym("_close_block")))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// HTML block type 6: block-level tag, ends at blank line
	g.Define("_html_block_6",
		Seq(
			Sym("_html_block_6_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_html_block_newline"),
				Seq(
					Seq(
						Sym("_html_block_newline"),
						Sym("_blank_line")),
					Sym("_close_block")))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// HTML block type 7: any complete open/close tag, ends at blank line
	g.Define("_html_block_7",
		Seq(
			Sym("_html_block_7_start"),
			Repeat(Choice(
				Sym("_line"),
				Sym("_html_block_newline"),
				Seq(
					Seq(
						Sym("_html_block_newline"),
						Sym("_blank_line")),
					Sym("_close_block")))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// link reference definition [label]: url "title"
	g.Define("link_reference_definition",
		PrecDynamic(10, Seq(
			Choice(
				Sym("_whitespace"),
				Blank()),
			Sym("link_label"),
			Str(":"),
			Choice(
				Seq(
					Choice(
						Sym("_whitespace"),
						Blank()),
					Choice(
						Seq(
							Sym("_soft_line_break"),
							Choice(
								Sym("_whitespace"),
								Blank())),
						Blank())),
				Blank()),
			Sym("link_destination"),
			Choice(
				PrecDynamic(20, Seq(
					Choice(
						Seq(
							Sym("_whitespace"),
							Choice(
								Seq(
									Sym("_soft_line_break"),
									Choice(
										Sym("_whitespace"),
										Blank())),
								Blank())),
						Seq(
							Sym("_soft_line_break"),
							Choice(
								Sym("_whitespace"),
								Blank()))),
					Choice(
						Sym("_no_indented_chunk"),
						Blank()),
					Sym("link_title"))),
				Blank()),
			Choice(
				Sym("_link_reference_definition_newline"),
				Sym("_soft_line_break"),
				Sym("_eof")))))

	// inline text characters that do not start a link
	g.Define("_text_inline_no_link",
		Choice(
			Sym("_word"),
			Sym("_whitespace"),
			Seq(
				Choice(
					Str("!"),
					Str("\""),
					Str("#"),
					Str("$"),
					Str("%"),
					Str("&"),
					Str("'"),
					Str("("),
					Str(")"),
					Str("*"),
					Str("+"),
					Str(","),
					Str("-"),
					Str("."),
					Str("/"),
					Str(":"),
					Str(";"),
					Str("<"),
					Str("="),
					Str(">"),
					Str("?"),
					Str("@"),
					Str("\\"),
					Str("^"),
					Str("_"),
					Str("`"),
					Str("{"),
					Str("|"),
					Str("}"),
					Str("~")),
				Choice(
					Sym("_last_token_punctuation"),
					Blank()))))

	// a paragraph of inline content
	g.Define("paragraph",
		Seq(
			Alias(Repeat1(Choice(
				Sym("_paragraph_line"),
				Sym("_paragraph_soft_line_break"))), "inline", true),
			Choice(
				Sym("_paragraph_newline"),
				Sym("_eof"))))

	// soft line break used only inside paragraph; distinct name keeps paragraph
	// LALR states isolated from other contexts that consume _soft_line_break
	// (link_label, link_title, link_reference_definition).
	g.Define("_paragraph_soft_line_break",
		Seq(
			Sym("_soft_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// newline terminating a paragraph; distinct name prevents the paragraph's
	// trailing newline state from merging with block_quote-internal continuation
	// states, which would otherwise cause block_quote to close prematurely after
	// the first inner block instead of accepting further `> ...` lines.
	g.Define("_paragraph_newline",
		Seq(
			Sym("_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// one or more blank lines (block separator)
	g.Define("_blank_line",
		Seq(
			Sym("_blank_line_start"),
			Choice(
				Sym("_blank_line_newline"),
				Sym("_eof"))))

	// block-quote introduced by > marker
	// Hidden by name — see _fenced_code_block comment.
	//
	// The Repeat over `_block_in_container` suffixes each child with an
	// optional `block_continuation` slot so the blank-then-`>` line between
	// sibling block children inside a quote has a parser-recognised place
	// to emit a visible continuation marker. The bundled blob emits one
	// `(block_continuation)` per inter-child boundary; this seq mirrors
	// that shape. NOTE: the scanner still prefers `_block_quote_start` over
	// `block_continuation` whenever both are in valid_symbols at the same
	// position, so the symptom from audit Gap 4 (a spurious extra
	// `block_quote_marker` between siblings) is not fully resolved by this
	// grammar change alone — it requires scanner-state coordination in
	// `grammars/markdown_scanner.go`. This change is forward-compatible:
	// the slot exists, so once the scanner is updated to emit
	// `block_continuation` here, the CST will start matching the bundled
	// shape without further grammar changes.
	g.Define("_block_quote",
		Seq(
			Alias(Sym("_block_quote_start"), "block_quote_marker", true),
			Choice(
				Sym("block_continuation"),
				Blank()),
			Repeat(Seq(
				Sym("_block_in_container"),
				Choice(
					Sym("block_continuation"),
					Blank()))),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// any block-level element as it appears nested inside a block_quote or list_item.
	// Block_quote, list, and fenced_code_block in this context emit visible wrappers
	// (matching the ref CST), unlike at top level where those wrappers are hidden.
	g.Define("_block_in_container",
		Choice(
			Alias(Sym("_setext_heading1"), "setext_heading", true),
			Alias(Sym("_setext_heading2"), "setext_heading", true),
			Sym("paragraph"),
			Sym("indented_code_block"),
			Alias(Sym("_block_quote"), "block_quote", true),
			Sym("thematic_break"),
			Alias(Sym("_list"), "list", true),
			Alias(Sym("_fenced_code_block"), "fenced_code_block", true),
			Sym("_blank_line"),
			Sym("html_block"),
			Sym("link_reference_definition"),
			Sym("_pipe_table"),
			Sym("section")))

	// an ordered or unordered list
	// Hidden by name — see _fenced_code_block comment. The bundled markdown.bin
	// parser does not emit a `list` node at top level; list_items appear directly
	// under section. Aliases on nested list items still produce visible `list`
	// wrappers where the ref CST shows them.
	g.Define("_list",
		PrecRight(0, Choice(
			Sym("_list_plus"),
			Sym("_list_minus"),
			Sym("_list_star"),
			Sym("_list_dot"),
			Sym("_list_parenthesis"))))

	// list with + marker items
	g.Define("_list_plus",
		PrecRight(0, Repeat1(Alias(Sym("_list_item_plus"), "list_item", true))))

	// list with - marker items
	g.Define("_list_minus",
		PrecRight(0, Repeat1(Alias(Sym("_list_item_minus"), "list_item", true))))

	// list with * marker items
	g.Define("_list_star",
		PrecRight(0, Repeat1(Alias(Sym("_list_item_star"), "list_item", true))))

	// list with N. marker items (ordered)
	g.Define("_list_dot",
		PrecRight(0, Repeat1(Alias(Sym("_list_item_dot"), "list_item", true))))

	// list with N) marker items (ordered)
	g.Define("_list_parenthesis",
		PrecRight(0, Repeat1(Alias(Sym("_list_item_parenthesis"), "list_item", true))))

	// + list marker (visible node)
	g.Define("list_marker_plus",
		Choice(
			Sym("_list_marker_plus"),
			Sym("_list_marker_plus_dont_interrupt")))

	// - list marker (visible node)
	g.Define("list_marker_minus",
		Choice(
			Sym("_list_marker_minus"),
			Sym("_list_marker_minus_dont_interrupt")))

	// * list marker (visible node)
	g.Define("list_marker_star",
		Choice(
			Sym("_list_marker_star"),
			Sym("_list_marker_star_dont_interrupt")))

	// N. ordered list marker (visible node)
	g.Define("list_marker_dot",
		Choice(
			Sym("_list_marker_dot"),
			Sym("_list_marker_dot_dont_interrupt")))

	// N) ordered list marker (visible node)
	g.Define("list_marker_parenthesis",
		Choice(
			Sym("_list_marker_parenthesis"),
			Sym("_list_marker_parenthesis_dont_interrupt")))

	// list item introduced by a + marker
	g.Define("_list_item_plus",
		Seq(
			Sym("list_marker_plus"),
			Choice(
				Sym("block_continuation"),
				Blank()),
			Sym("_list_item_content"),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// list item introduced by a - marker
	g.Define("_list_item_minus",
		Seq(
			Sym("list_marker_minus"),
			Choice(
				Sym("block_continuation"),
				Blank()),
			Sym("_list_item_content"),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// list item introduced by a * marker
	g.Define("_list_item_star",
		Seq(
			Sym("list_marker_star"),
			Choice(
				Sym("block_continuation"),
				Blank()),
			Sym("_list_item_content"),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// list item introduced by a N. marker
	g.Define("_list_item_dot",
		Seq(
			Sym("list_marker_dot"),
			Choice(
				Sym("block_continuation"),
				Blank()),
			Sym("_list_item_content"),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// list item introduced by a N) marker
	g.Define("_list_item_parenthesis",
		Seq(
			Sym("list_marker_parenthesis"),
			Choice(
				Sym("block_continuation"),
				Blank()),
			Sym("_list_item_content"),
			Sym("_block_close"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// the block content of a single list item
	//
	// Uses `_block_in_list_item` instead of `_block_in_container` so the
	// LALR state for a list item's block body is distinct from the one
	// used inside `_block_quote`. Without this separation, multi-paragraph
	// list items see their second-and-later paragraphs hoisted out of the
	// outer `list_item` wrapper. Mirrors the `_paragraph_line` vs `_line`
	// pattern used elsewhere in this grammar to prevent LALR merge
	// artifacts (see audit Gap 3).
	//
	// NOTE: this partially fixes Gap 3 — multi-paragraph items now keep
	// their outer wrapper. Single-item lists with non-trivial body (lazy
	// continuation, nested list, nested container) still hoist the outer
	// `list_item` wrapper. That residual problem is a grammargen-level
	// single-iteration Repeat1 reduction artifact (the alias on
	// `_list_item_minus` inside `Repeat1` inlines when there is exactly
	// one iteration), not a markdown-grammar issue, and needs a fix in
	// grammargen/normalize.go's auxiliary-rule lowering.
	g.Define("_list_item_content",
		Choice(
			Prec(1, Seq(
				Sym("_blank_line"),
				Sym("_blank_line"),
				Sym("_close_block"),
				Choice(
					Sym("block_continuation"),
					Blank()))),
			Repeat1(Sym("_block_in_list_item")),
			Prec(1, Seq(
				Choice(
					Sym("task_list_marker_checked"),
					Sym("task_list_marker_unchecked")),
				Sym("_whitespace"),
				Sym("paragraph"),
				Repeat(Sym("_block_in_list_item"))))))

	// Structurally-identical copy of `_block_in_container`, used inside
	// `_list_item_content`. The separation gives the LALR generator a
	// distinct state for list-item block bodies so it does not merge with
	// the block_quote block-body state — merging causes the outer
	// `list_item` alias to evaporate whenever the item body contains more
	// than a single paragraph. See audit Gap 3 for inputs and analysis:
	// inbox/agents/2026-05-28-grammargen-wrapper-gap-audit.md.
	g.Define("_block_in_list_item",
		Choice(
			Alias(Sym("_setext_heading1"), "setext_heading", true),
			Alias(Sym("_setext_heading2"), "setext_heading", true),
			Sym("paragraph"),
			Sym("indented_code_block"),
			Alias(Sym("_block_quote"), "block_quote", true),
			Sym("thematic_break"),
			Alias(Sym("_list"), "list", true),
			Alias(Sym("_fenced_code_block"), "fenced_code_block", true),
			Sym("_blank_line"),
			Sym("html_block"),
			Sym("link_reference_definition"),
			Sym("_pipe_table"),
			Sym("section")))

	// external newline (inline continuation context)
	g.Define("_newline",
		Seq(
			Sym("_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// soft line break inside inline content
	g.Define("_soft_line_break",
		Seq(
			Sym("_soft_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// newline inside an indented code chunk — structurally identical to
	// _newline but with a distinct rule name so its LR items remain separate
	// from paragraph's _soft_line_break states in the LALR automaton.
	g.Define("_indented_chunk_newline",
		Seq(
			Sym("_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// newline between the pipe_table header row and the delimiter row.
	// newline inside HTML block body — distinct name prevents LALR merging
	// with block-dispatch states that have _html_block_*_start valid, which
	// would otherwise leak those tokens into paragraph-continuation states.
	g.Define("_html_block_newline",
		Seq(
			Sym("_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// newline inside fenced_code_block content — distinct name prevents LALR
	// merging with _newline contexts where _close_block IS a valid lookahead
	// (e.g. _blank_line → _blank_line_start _newline followed by html_block 6/7
	// → ... Seq(_html_block_newline, _blank_line) _close_block). Without this
	// split, state-209-equivalent merges across contexts and the external
	// scanner fires _close_block prematurely inside fenced code content. Body
	// identical to _newline; only the name differs (same pattern as
	// _indented_chunk_newline and _html_block_newline introduced by fbc52a58).
	g.Define("_fenced_code_block_newline",
		Seq(
			Sym("_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// newline terminating a link_reference_definition — structurally identical
	// to _newline but with a distinct rule name so its LR items stay separate
	// from the fenced_code_block-body _newline/_close_block context. Without
	// this split, the link-ref-def boundary state and the fence-body close
	// state merge into a single LALR state whose valid_symbols include both
	// block_continuation (shift) and _close_block; the bundled scanner's eager
	// _close_block then either collapses consecutive link-ref-defs into a
	// paragraph (if _close_block is offered) or eats the fence body (if it is
	// suppressed). Splitting the terminator gives each context its own state,
	// so the choice is unambiguous and no scanner-state suppression is needed.
	// Same de-merge pattern as _paragraph_newline / _fenced_code_block_newline.
	g.Define("_link_reference_definition_newline",
		Seq(
			Sym("_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// newline closing a _blank_line — structurally identical to _newline but
	// distinct so the blank-line-after-a-block state (e.g. paragraph then blank
	// then link_reference_definition) does not merge with the fenced_code_block
	// body-close state. Both otherwise reduce the shared _newline with a
	// block_continuation shift and collapse into one LALR state where the
	// bundled scanner's eager _close_block can't be cleanly suppressed for one
	// without breaking the other. Same de-merge pattern as _paragraph_newline.
	g.Define("_blank_line_newline",
		Seq(
			Sym("_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// a single line of text (paragraph continuation)
	g.Define("_line",
		PrecRight(0, Repeat1(Choice(
			Sym("_word"),
			Sym("_whitespace"),
			Seq(
				Choice(
					Str("!"),
					Str("\""),
					Str("#"),
					Str("$"),
					Str("%"),
					Str("&"),
					Str("'"),
					Str("("),
					Str(")"),
					Str("*"),
					Str("+"),
					Str(","),
					Str("-"),
					Str("."),
					Str("/"),
					Str(":"),
					Str(";"),
					Str("<"),
					Str("="),
					Str(">"),
					Str("?"),
					Str("@"),
					Str("["),
					Str("\\"),
					Str("]"),
					Str("^"),
					Str("_"),
					Str("`"),
					Str("{"),
					Str("|"),
					Str("}"),
					Str("~")),
				Choice(
					Sym("_last_token_punctuation"),
					Blank()))))))

	// A structurally-identical copy of _line for use inside code_fence_content.
	// Using a distinct rule name keeps `code_fence_content`'s content states
	// from merging with paragraph `_line` states. Without the split, fences
	// after non-paragraph blocks (heading, blank line, etc.) re-enter a merged
	// content state where `_close_block` is in the reduce-lookahead set, and
	// the scanner's eager `_close_block` branch (markdown_scanner.go line 344)
	// fires before the body is parsed — so the body gets reparsed as a
	// `paragraph` and the closing ``` becomes a fresh fence wrapper. The body
	// is identical to `_line`; only the name differs (same isolation pattern
	// as `_paragraph_line` and `_fenced_code_block_newline`).
	g.Define("_fenced_code_block_line",
		PrecRight(0, Repeat1(Choice(
			Sym("_word"),
			Sym("_whitespace"),
			Seq(
				Choice(
					Str("!"),
					Str("\""),
					Str("#"),
					Str("$"),
					Str("%"),
					Str("&"),
					Str("'"),
					Str("("),
					Str(")"),
					Str("*"),
					Str("+"),
					Str(","),
					Str("-"),
					Str("."),
					Str("/"),
					Str(":"),
					Str(";"),
					Str("<"),
					Str("="),
					Str(">"),
					Str("?"),
					Str("@"),
					Str("["),
					Str("\\"),
					Str("]"),
					Str("^"),
					Str("_"),
					Str("`"),
					Str("{"),
					Str("|"),
					Str("}"),
					Str("~")),
				Choice(
					Sym("_last_token_punctuation"),
					Blank()))))))

	// A structurally-identical copy of _line for use inside paragraph.
	// Using a distinct rule name prevents LALR state merging between paragraph
	// (which allows _soft_line_ending after each line) and other block contexts
	// (_indented_chunk, html_block, code_fence_content) that must NOT allow it.
	// _line is used in all non-paragraph contexts; _paragraph_line is used only
	// in paragraph, so _soft_line_ending stays out of _line_repeat1's FOLLOW set.
	g.Define("_paragraph_line",
		PrecRight(0, Repeat1(Choice(
			Sym("_word"),
			Sym("_whitespace"),
			Seq(
				Choice(
					Str("!"),
					Str("\""),
					Str("#"),
					Str("$"),
					Str("%"),
					Str("&"),
					Str("'"),
					Str("("),
					Str(")"),
					Str("*"),
					Str("+"),
					Str(","),
					Str("-"),
					Str("."),
					Str("/"),
					Str(":"),
					Str(";"),
					Str("<"),
					Str("="),
					Str(">"),
					Str("?"),
					Str("@"),
					Str("["),
					Str("\\"),
					Str("]"),
					Str("^"),
					Str("_"),
					Str("`"),
					Str("{"),
					Str("|"),
					Str("}"),
					Str("~")),
				Choice(
					Sym("_last_token_punctuation"),
					Blank()))))))

	// a run of non-whitespace, non-special characters
	g.Define("_word",
		Choice(
			Pat("[^!-/:-@\\[-`\\{-~ \\t\\n\\r]+"),
			Choice(
				Pat(`\[[xX]\]`),
				Pat(`\[[ \t]\]`))))

	// horizontal whitespace (spaces and tabs)
	g.Define("_whitespace",
		Pat(`[ \t]+`))

	// GFM task list checked marker [x]
	g.Define("task_list_marker_checked",
		Prec(1, Pat(`\[[xX]\]`)))

	// GFM task list unchecked marker [ ]
	g.Define("task_list_marker_unchecked",
		Prec(1, Pat(`\[[ \t]\]`)))

	// GFM pipe table (header + delimiter row + data rows)
	// Structure matches the reference grammar exactly:
	//   _pipe_table_start alias(pipe_table_row,"pipe_table_header") _newline
	//   pipe_table_delimiter_row Repeat(Seq(_pipe_table_newline, Choice(row,Blank)))
	//   Choice(_newline, _eof)
	g.Define("_pipe_table_header_block",
		Seq(
			Sym("_pipe_table_start"),
			Alias(Sym("pipe_table_row"), "pipe_table_header", true),
			Sym("_newline")))

	// Hidden by name — see _fenced_code_block comment.
	g.Define("_pipe_table",
		PrecRight(0, Seq(
			Sym("_pipe_table_header_block"),
			Sym("pipe_table_delimiter_row"),
			Repeat(Seq(
				Sym("_pipe_table_newline"),
				Choice(
					Sym("pipe_table_row"),
					Blank()))),
			Choice(
				Sym("_newline"),
				Sym("_eof")))))

	// newline inside a pipe table
	g.Define("_pipe_table_newline",
		Seq(
			Sym("_pipe_table_line_ending"),
			Choice(
				Sym("block_continuation"),
				Blank())))

	// pipe table delimiter row (---)
	g.Define("pipe_table_delimiter_row",
		Seq(
			Choice(
				Seq(
					Choice(
						Sym("_whitespace"),
						Blank()),
					Str("|")),
				Blank()),
			Repeat1(PrecRight(0, Seq(
				Choice(
					Sym("_whitespace"),
					Blank()),
				Sym("pipe_table_delimiter_cell"),
				Choice(
					Sym("_whitespace"),
					Blank()),
				Str("|")))),
			Choice(
				Sym("_whitespace"),
				Blank()),
			Choice(
				Seq(
					Sym("pipe_table_delimiter_cell"),
					Choice(
						Sym("_whitespace"),
						Blank())),
				Blank())))

	// a single cell in the pipe table delimiter row
	g.Define("pipe_table_delimiter_cell",
		Seq(
			Choice(
				Alias(Str(":"), "pipe_table_align_left", true),
				Blank()),
			Repeat1(Str("-")),
			Choice(
				Alias(Str(":"), "pipe_table_align_right", true),
				Blank())))

	// a data row in a pipe table
	g.Define("pipe_table_row",
		Seq(
			Choice(
				Seq(
					Choice(
						Sym("_whitespace"),
						Blank()),
					Str("|")),
				Blank()),
			Choice(
				Seq(
					Repeat1(PrecRight(0, Seq(
						Choice(
							Seq(
								Choice(
									Sym("_whitespace"),
									Blank()),
								Sym("pipe_table_cell"),
								Choice(
									Sym("_whitespace"),
									Blank())),
							Alias(Sym("_whitespace"), "pipe_table_cell", true)),
						Str("|")))),
					Choice(
						Sym("_whitespace"),
						Blank()),
					Choice(
						Seq(
							Sym("pipe_table_cell"),
							Choice(
								Sym("_whitespace"),
								Blank())),
						Blank())),
				Seq(
					Choice(
						Sym("_whitespace"),
						Blank()),
					Sym("pipe_table_cell"),
					Choice(
						Sym("_whitespace"),
						Blank())))))

	// a single cell in a pipe table data row
	g.Define("pipe_table_cell",
		PrecRight(0, Seq(
			Choice(
				Sym("_word"),
				Sym("_backslash_escape"),
				Seq(
					Choice(
						Str("!"),
						Str("\""),
						Str("#"),
						Str("$"),
						Str("%"),
						Str("&"),
						Str("'"),
						Str("("),
						Str(")"),
						Str("*"),
						Str("+"),
						Str(","),
						Str("-"),
						Str("."),
						Str("/"),
						Str(":"),
						Str(";"),
						Str("<"),
						Str("="),
						Str(">"),
						Str("?"),
						Str("@"),
						Str("["),
						Str("\\"),
						Str("]"),
						Str("^"),
						Str("_"),
						Str("`"),
						Str("{"),
						Str("}"),
						Str("~")),
					Choice(
						Sym("_last_token_punctuation"),
						Blank()))),
			Repeat(Choice(
				Sym("_word"),
				Sym("_whitespace"),
				Sym("_backslash_escape"),
				Seq(
					Choice(
						Str("!"),
						Str("\""),
						Str("#"),
						Str("$"),
						Str("%"),
						Str("&"),
						Str("'"),
						Str("("),
						Str(")"),
						Str("*"),
						Str("+"),
						Str(","),
						Str("-"),
						Str("."),
						Str("/"),
						Str(":"),
						Str(";"),
						Str("<"),
						Str("="),
						Str(">"),
						Str("?"),
						Str("@"),
						Str("["),
						Str("\\"),
						Str("]"),
						Str("^"),
						Str("_"),
						Str("`"),
						Str("{"),
						Str("}"),
						Str("~")),
					Choice(
						Sym("_last_token_punctuation"),
						Blank())))))))

	// externals: 47 tokens owned by the hand-written external scanner.
	g.SetExternals(
		Sym("_line_ending"),
		Sym("_soft_line_ending"),
		Sym("_block_close"),
		Sym("block_continuation"),
		Sym("_block_quote_start"),
		Sym("_indented_chunk_start"),
		Sym("atx_h1_marker"),
		Sym("atx_h2_marker"),
		Sym("atx_h3_marker"),
		Sym("atx_h4_marker"),
		Sym("atx_h5_marker"),
		Sym("atx_h6_marker"),
		Sym("setext_h1_underline"),
		Sym("setext_h2_underline"),
		Sym("_thematic_break"),
		Sym("_list_marker_minus"),
		Sym("_list_marker_plus"),
		Sym("_list_marker_star"),
		Sym("_list_marker_parenthesis"),
		Sym("_list_marker_dot"),
		Sym("_list_marker_minus_dont_interrupt"),
		Sym("_list_marker_plus_dont_interrupt"),
		Sym("_list_marker_star_dont_interrupt"),
		Sym("_list_marker_parenthesis_dont_interrupt"),
		Sym("_list_marker_dot_dont_interrupt"),
		Sym("_fenced_code_block_start_backtick"),
		Sym("_fenced_code_block_start_tilde"),
		Sym("_blank_line_start"),
		Sym("_fenced_code_block_end_backtick"),
		Sym("_fenced_code_block_end_tilde"),
		Sym("_html_block_1_start"),
		Sym("_html_block_1_end"),
		Sym("_html_block_2_start"),
		Sym("_html_block_3_start"),
		Sym("_html_block_4_start"),
		Sym("_html_block_5_start"),
		Sym("_html_block_6_start"),
		Sym("_html_block_7_start"),
		Sym("_close_block"),
		Sym("_no_indented_chunk"),
		Sym("_error"),
		Sym("_trigger_error"),
		Sym("_eof"),
		Sym("minus_metadata"),
		Sym("plus_metadata"),
		Sym("_pipe_table_start"),
		Sym("_pipe_table_line_ending"),
	)

	// conflicts: parser conflicts resolved by GLR or dynamic precedence.
	g.SetConflicts(
		[]string{"link_reference_definition"},
		[]string{"link_label", "_paragraph_line"},
		[]string{"link_reference_definition", "_paragraph_line"},
	)

	// Suppress LALR reduce-lookahead artifacts: when an external token only has
	// reduce actions that duplicate non-external reduce actions in the same state,
	// suppress the external token. This prevents _soft_line_ending from leaking
	// into _indented_chunk states via LALR FOLLOW-set propagation.
	g.SuppressEquivalentExternalReduceLookaheads = true

	// precedences: symbol-level precedence ordering for ambiguous constructs.
	g.Precedences = [][]PrecEntry{
		{
			{IsSymbol: true, Name: "_setext_heading1"},
			{IsSymbol: true, Name: "_block"},
		},
		{
			{IsSymbol: true, Name: "_setext_heading2"},
			{IsSymbol: true, Name: "_block"},
		},
		{
			{IsSymbol: true, Name: "indented_code_block"},
			{IsSymbol: true, Name: "_block"},
		},
	}

	return g
}

