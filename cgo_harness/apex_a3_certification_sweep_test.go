//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
)

// TestApexA3CompactCertificationFullCorpusSweep is the A3
// certification-workstream (spec.campaign.v7, finding
// tied-election-family-compact-retirement) full-corpus verification receipt
// for Apex. Apex has no corpus_real directory (it is not one of the top-50
// languages cgo_harness/corpus_real's manifest profiles), so this sweep uses
// the smoke sample, the three existing tied-election witnesses
// (apex_generic_local_parity_test.go), and 20 constructed adversarial
// sources exercising Apex's conflict-heavy shapes (generics, SOQL/SOSL,
// sharing, triggers, casts, switch-on, annotations).
func TestApexA3CompactCertificationFullCorpusSweep(t *testing.T) {
	lang := grammars.ApexLanguage()
	if !lang.CompactPrimaryAcceptanceDerivationCertified {
		t.Fatal("apex did not receive the A3 primary-acceptance-derivation certification")
	}
	if lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("apex unexpectedly carries the converged-split-drop certification (apex does not need it)")
	}

	sources := []a3CertificationSweepSource{
		{Name: "smoke_sample", Source: []byte(grammars.ParseSmokeSample("apex"))},
	}
	sources = append(sources, apexA3TiedElectionWitnesses()...)
	sources = append(sources, apexA3AdversarialSources()...)

	result := runA3CertificationSweep(t, "apex", "apex", lang, sources)
	a3ReportSweep(t, result)
}

// apexA3TiedElectionWitnesses reuses the three witnesses already vetted in
// apex_generic_local_parity_test.go (TestApexCloseAngleRawCOracleParity),
// including the tied class-literal election the finding names ("apex
// class-literal").
func apexA3TiedElectionWitnesses() []a3CertificationSweepSource {
	return []a3CertificationSweepSource{
		{
			Name: "nested_generic_local",
			Source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    List<List<SObject>> searchResults = [FIND :keyword IN ALL FIELDS];\n" +
				"  }\n" +
				"}\n"),
		},
		{
			Name: "right_shift",
			Source: []byte("public class C {\n" +
				"  Integer m(Integer value) {\n" +
				"    return value >> 1;\n" +
				"  }\n" +
				"}\n"),
		},
		{
			// Issue #601 tied election: class_literal vs field_access. The
			// finding's "apex class-literal" witness -- the compact route is
			// strictly more faithful than production plus the compat arm here.
			Name: "class_literal_alias",
			Source: []byte("public class C {\n" +
				"  void m() {\n" +
				"    Object t = RecordPage.class;\n" +
				"  }\n" +
				"}\n"),
		},
	}
}

// apexA3AdversarialSources constructs 20 sources exercising Apex's
// conflict-heavy grammar shapes: nested generics, SOQL/SOSL query
// expressions, sharing and trigger declarations, casts versus parenthesized
// expressions, switch-on, annotations, and property accessors.
func apexA3AdversarialSources() []a3CertificationSweepSource {
	return []a3CertificationSweepSource{
		{Name: "soql_basic_query", Source: []byte(
			"public class C {\n" +
				"  List<Account> m() {\n" +
				"    return [SELECT Id, Name FROM Account WHERE Name = 'Acme' LIMIT 10];\n" +
				"  }\n" +
				"}\n")},
		{Name: "soql_nested_subquery", Source: []byte(
			"public class C {\n" +
				"  List<Account> m() {\n" +
				"    return [SELECT Id, (SELECT Id, LastName FROM Contacts) FROM Account];\n" +
				"  }\n" +
				"}\n")},
		{Name: "sosl_returning_query", Source: []byte(
			"public class C {\n" +
				"  void m() {\n" +
				"    List<List<SObject>> results = [FIND 'test' IN ALL FIELDS RETURNING Account(Id, Name), Contact(Id)];\n" +
				"  }\n" +
				"}\n")},
		{Name: "with_sharing_class", Source: []byte(
			"public with sharing class C {\n" +
				"  public void m() {}\n" +
				"}\n")},
		{Name: "without_sharing_class", Source: []byte(
			"public without sharing class C {\n" +
				"  public void m() {}\n" +
				"}\n")},
		{Name: "inherited_sharing_class", Source: []byte(
			"public inherited sharing class C {\n" +
				"  public void m() {}\n" +
				"}\n")},
		{Name: "trigger_before_insert", Source: []byte(
			"trigger AccountTrigger on Account (before insert, before update) {\n" +
				"  for (Account a : Trigger.new) {\n" +
				"    a.Name = a.Name.trim();\n" +
				"  }\n" +
				"}\n")},
		{Name: "interface_declaration", Source: []byte(
			"public interface Greeter {\n" +
				"  String greet(String name);\n" +
				"}\n")},
		{Name: "enum_declaration", Source: []byte(
			"public enum Season { WINTER, SPRING, SUMMER, FALL }\n")},
		{Name: "static_initializer_block", Source: []byte(
			"public class C {\n" +
				"  static Integer counter;\n" +
				"  static {\n" +
				"    counter = 0;\n" +
				"  }\n" +
				"}\n")},
		{Name: "try_catch_finally_multi", Source: []byte(
			"public class C {\n" +
				"  void m() {\n" +
				"    try {\n" +
				"      Integer x = 1 / 0;\n" +
				"    } catch (DmlException e) {\n" +
				"      System.debug(e.getMessage());\n" +
				"    } catch (Exception e) {\n" +
				"      System.debug('other');\n" +
				"    } finally {\n" +
				"      System.debug('done');\n" +
				"    }\n" +
				"  }\n" +
				"}\n")},
		{Name: "annotation_istest", Source: []byte(
			"@isTest\n" +
				"private class CTest {\n" +
				"  @isTest\n" +
				"  static void testMethod1() {\n" +
				"    System.assertEquals(1, 1);\n" +
				"  }\n" +
				"}\n")},
		{Name: "aura_enabled_property", Source: []byte(
			"public class C {\n" +
				"  @AuraEnabled\n" +
				"  public Integer count { get; set; }\n" +
				"}\n")},
		{Name: "cast_vs_parenthesized_expr", Source: []byte(
			"public class C {\n" +
				"  void m(Object obj) {\n" +
				"    Account a = (Account)obj;\n" +
				"    Integer x = (1 + 2);\n" +
				"  }\n" +
				"}\n")},
		{Name: "switch_on_statement", Source: []byte(
			"public class C {\n" +
				"  String m(Integer x) {\n" +
				"    switch on x {\n" +
				"      when 1 {\n" +
				"        return 'one';\n" +
				"      }\n" +
				"      when 2, 3 {\n" +
				"        return 'two-or-three';\n" +
				"      }\n" +
				"      when else {\n" +
				"        return 'other';\n" +
				"      }\n" +
				"    }\n" +
				"    return null;\n" +
				"  }\n" +
				"}\n")},
		{Name: "instanceof_and_generic_collection", Source: []byte(
			"public class C {\n" +
				"  void m(Object obj) {\n" +
				"    if (obj instanceof Map<String, List<Integer>>) {\n" +
				"      System.debug('map');\n" +
				"    }\n" +
				"  }\n" +
				"}\n")},
		{Name: "nested_class_and_generic_method", Source: []byte(
			"public class Outer {\n" +
				"  public class Inner {\n" +
				"    public Integer value;\n" +
				"  }\n" +
				"  public static <T> List<T> wrap(T item) {\n" +
				"    List<T> result = new List<T>();\n" +
				"    result.add(item);\n" +
				"    return result;\n" +
				"  }\n" +
				"}\n")},
		{Name: "ternary_with_nested_generics", Source: []byte(
			"public class C {\n" +
				"  Map<String, List<Integer>> m(Boolean flag) {\n" +
				"    return flag ? new Map<String, List<Integer>>() : null;\n" +
				"  }\n" +
				"}\n")},
		{Name: "for_each_soql_inline", Source: []byte(
			"public class C {\n" +
				"  void m() {\n" +
				"    for (Account a : [SELECT Id FROM Account WHERE Name != null]) {\n" +
				"      System.debug(a.Id);\n" +
				"    }\n" +
				"  }\n" +
				"}\n")},
		{Name: "bind_variable_soql_with_generic_local", Source: []byte(
			"public class C {\n" +
				"  void m(String keyword) {\n" +
				"    List<Account> results = [SELECT Id FROM Account WHERE Name = :keyword];\n" +
				"    List<List<SObject>> both = [FIND :keyword IN ALL FIELDS RETURNING Account, Contact];\n" +
				"  }\n" +
				"}\n")},
	}
}
