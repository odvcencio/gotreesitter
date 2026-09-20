package grammargen

// YAMLGrammar returns the owned YAML grammar.
//
// Baseline: tree-sitter-grammars/tree-sitter-yaml a1c4812a73ec5e089de8e441fdea3a921e8d5079.
// This Go DSL is the canonical gotreesitter source for YAML grammar changes.
func YAMLGrammar() *Grammar {
	g := NewGrammar("yaml")

	g.Define("stream",
		Seq(
			Choice(
				Choice(
					Seq(
						Choice(
							Alias(
								Sym("_bgn_imp_doc"),
								"document", true,
							),
							Alias(
								Sym("_drs_doc"),
								"document", true,
							),
							Alias(
								Sym("_exp_doc"),
								"document", true,
							),
						),
						Choice(
							Choice(
								Sym("_doc_w_bgn_w_end_seq"),
								Sym("_doc_w_bgn_wo_end_seq"),
							),
							Blank(),
						),
					),
					Seq(
						Choice(
							Alias(
								Sym("_bgn_imp_doc_end"),
								"document", true,
							),
							Alias(
								Sym("_drs_doc_end"),
								"document", true,
							),
							Alias(
								Sym("_exp_doc_end"),
								"document", true,
							),
							Alias(
								Sym("_doc_end"),
								"document", true,
							),
						),
						Choice(
							Choice(
								Sym("_doc_w_bgn_w_end_seq"),
								Sym("_doc_w_bgn_wo_end_seq"),
								Sym("_doc_wo_bgn_w_end_seq"),
								Sym("_doc_wo_bgn_wo_end_seq"),
							),
							Blank(),
						),
					),
				),
				Blank(),
			),
			Sym("_eof"),
		))

	g.Define("_doc_w_bgn_w_end_seq",
		Seq(
			Sym("_doc_w_bgn_w_end"),
			Choice(
				Choice(
					Sym("_doc_w_bgn_w_end_seq"),
					Sym("_doc_w_bgn_wo_end_seq"),
					Sym("_doc_wo_bgn_w_end_seq"),
					Sym("_doc_wo_bgn_wo_end_seq"),
				),
				Blank(),
			),
		))

	g.Define("_doc_w_bgn_wo_end_seq",
		Seq(
			Sym("_doc_w_bgn_wo_end"),
			Choice(
				Choice(
					Sym("_doc_w_bgn_w_end_seq"),
					Sym("_doc_w_bgn_wo_end_seq"),
				),
				Blank(),
			),
		))

	g.Define("_doc_wo_bgn_w_end_seq",
		Seq(
			Sym("_doc_wo_bgn_w_end"),
			Choice(
				Choice(
					Sym("_doc_w_bgn_w_end_seq"),
					Sym("_doc_w_bgn_wo_end_seq"),
					Sym("_doc_wo_bgn_w_end_seq"),
					Sym("_doc_wo_bgn_wo_end_seq"),
				),
				Blank(),
			),
		))

	g.Define("_doc_wo_bgn_wo_end_seq",
		Seq(
			Sym("_doc_wo_bgn_wo_end"),
			Choice(
				Choice(
					Sym("_doc_w_bgn_w_end_seq"),
					Sym("_doc_w_bgn_wo_end_seq"),
				),
				Blank(),
			),
		))

	g.Define("_doc_w_bgn_w_end",
		Choice(
			Alias(
				Sym("_exp_doc_end"),
				"document", true,
			),
			Alias(
				Sym("_doc_end"),
				"document", true,
			),
		))

	g.Define("_doc_w_bgn_wo_end",
		Alias(
			Sym("_exp_doc"),
			"document", true,
		))

	g.Define("_doc_wo_bgn_w_end",
		Choice(
			Alias(
				Sym("_drs_doc_end"),
				"document", true,
			),
			Alias(
				Sym("_imp_doc_end"),
				"document", true,
			),
		))

	g.Define("_doc_wo_bgn_wo_end",
		Choice(
			Alias(
				Sym("_drs_doc"),
				"document", true,
			),
			Alias(
				Sym("_imp_doc"),
				"document", true,
			),
		))

	g.Define("_bgn_imp_doc",
		Choice(
			Sym("_exp_doc_tal"),
			Alias(
				Sym("_r_blk_seq_r_val"),
				"block_node", true,
			),
			Alias(
				Sym("_r_blk_map_r_val"),
				"block_node", true,
			),
		))

	g.Define("_drs_doc",
		Seq(
			Repeat1(
				Sym("_s_dir"),
			),
			Sym("_exp_doc"),
		))

	g.Define("_exp_doc",
		Seq(
			Alias(
				Sym("_s_drs_end"),
				"---", false,
			),
			Choice(
				Sym("_exp_doc_tal"),
				Blank(),
			),
		))

	g.Define("_imp_doc",
		Choice(
			Alias(
				Sym("_br_blk_seq_val"),
				"block_node", true,
			),
			Alias(
				Sym("_br_blk_map_val"),
				"block_node", true,
			),
			Alias(
				Sym("_br_blk_str_val"),
				"block_node", true,
			),
			Sym("_br_flw_val_blk"),
		))

	g.Define("_drs_doc_end",
		Prec(1,
			Seq(
				Sym("_drs_doc"),
				Alias(
					Sym("_s_doc_end"),
					"...", false,
				),
			),
		))

	g.Define("_exp_doc_end",
		Prec(1,
			Seq(
				Sym("_exp_doc"),
				Alias(
					Sym("_s_doc_end"),
					"...", false,
				),
			),
		))

	g.Define("_imp_doc_end",
		Prec(1,
			Seq(
				Sym("_imp_doc"),
				Alias(
					Sym("_s_doc_end"),
					"...", false,
				),
			),
		))

	g.Define("_bgn_imp_doc_end",
		Prec(1,
			Seq(
				Sym("_bgn_imp_doc"),
				Alias(
					Sym("_s_doc_end"),
					"...", false,
				),
			),
		))

	g.Define("_doc_end",
		Alias(
			Sym("_s_doc_end"),
			"...", false,
		))

	g.Define("_exp_doc_tal",
		Choice(
			Alias(
				Sym("_r_blk_seq_br_val"),
				"block_node", true,
			),
			Alias(
				Sym("_br_blk_seq_val"),
				"block_node", true,
			),
			Alias(
				Sym("_r_blk_map_br_val"),
				"block_node", true,
			),
			Alias(
				Sym("_br_blk_map_val"),
				"block_node", true,
			),
			Alias(
				Sym("_r_blk_str_val"),
				"block_node", true,
			),
			Alias(
				Sym("_br_blk_str_val"),
				"block_node", true,
			),
			Sym("_r_flw_val_blk"),
			Sym("_br_flw_val_blk"),
		))

	g.Define("_s_dir",
		Choice(
			Alias(
				Sym("_s_dir_yml"),
				"yaml_directive", true,
			),
			Alias(
				Sym("_s_dir_tag"),
				"tag_directive", true,
			),
			Alias(
				Sym("_s_dir_rsv"),
				"reserved_directive", true,
			),
		))

	g.Define("_s_dir_yml",
		Seq(
			Sym("_s_dir_yml_bgn"),
			Alias(
				Sym("_r_dir_yml_ver"),
				"yaml_version", true,
			),
		))

	g.Define("_s_dir_tag",
		Seq(
			Sym("_s_dir_tag_bgn"),
			Alias(
				Sym("_r_dir_tag_hdl"),
				"tag_handle", true,
			),
			Alias(
				Sym("_r_dir_tag_pfx"),
				"tag_prefix", true,
			),
		))

	g.Define("_s_dir_rsv",
		Seq(
			Alias(
				Sym("_s_dir_rsv_bgn"),
				"directive_name", true,
			),
			Repeat(
				Alias(
					Sym("_r_dir_rsv_prm"),
					"directive_parameter", true,
				),
			),
		))

	g.Define("_r_prp_val",
		Sym("_r_prp"))

	g.Define("_br_prp_val",
		Sym("_br_prp"))

	g.Define("_r_sgl_prp_val",
		Sym("_r_sgl_prp"))

	g.Define("_br_sgl_prp_val",
		Sym("_br_sgl_prp"))

	g.Define("_b_sgl_prp_val",
		Sym("_b_sgl_prp"))

	g.Define("_r_prp",
		Choice(
			Seq(
				Alias(
					Sym("_r_acr"),
					"anchor", true,
				),
				Choice(
					Choice(
						Alias(
							Sym("_r_tag"),
							"tag", true,
						),
						Alias(
							Sym("_br_tag"),
							"tag", true,
						),
					),
					Blank(),
				),
			),
			Seq(
				Alias(
					Sym("_r_tag"),
					"tag", true,
				),
				Choice(
					Choice(
						Alias(
							Sym("_r_acr"),
							"anchor", true,
						),
						Alias(
							Sym("_br_acr"),
							"anchor", true,
						),
					),
					Blank(),
				),
			),
		))

	g.Define("_br_prp",
		Choice(
			Seq(
				Alias(
					Sym("_br_acr"),
					"anchor", true,
				),
				Choice(
					Choice(
						Alias(
							Sym("_r_tag"),
							"tag", true,
						),
						Alias(
							Sym("_br_tag"),
							"tag", true,
						),
					),
					Blank(),
				),
			),
			Seq(
				Alias(
					Sym("_br_tag"),
					"tag", true,
				),
				Choice(
					Choice(
						Alias(
							Sym("_r_acr"),
							"anchor", true,
						),
						Alias(
							Sym("_br_acr"),
							"anchor", true,
						),
					),
					Blank(),
				),
			),
		))

	g.Define("_r_sgl_prp",
		Choice(
			Seq(
				Alias(
					Sym("_r_acr"),
					"anchor", true,
				),
				Choice(
					Alias(
						Sym("_r_tag"),
						"tag", true,
					),
					Blank(),
				),
			),
			Seq(
				Alias(
					Sym("_r_tag"),
					"tag", true,
				),
				Choice(
					Alias(
						Sym("_r_acr"),
						"anchor", true,
					),
					Blank(),
				),
			),
		))

	g.Define("_br_sgl_prp",
		Choice(
			Seq(
				Alias(
					Sym("_br_acr"),
					"anchor", true,
				),
				Choice(
					Alias(
						Sym("_r_tag"),
						"tag", true,
					),
					Blank(),
				),
			),
			Seq(
				Alias(
					Sym("_br_tag"),
					"tag", true,
				),
				Choice(
					Alias(
						Sym("_r_acr"),
						"anchor", true,
					),
					Blank(),
				),
			),
		))

	g.Define("_b_sgl_prp",
		Choice(
			Seq(
				Alias(
					Sym("_b_acr"),
					"anchor", true,
				),
				Choice(
					Alias(
						Sym("_r_tag"),
						"tag", true,
					),
					Blank(),
				),
			),
			Seq(
				Alias(
					Sym("_b_tag"),
					"tag", true,
				),
				Choice(
					Alias(
						Sym("_r_acr"),
						"anchor", true,
					),
					Blank(),
				),
			),
		))

	g.Define("_r_blk_seq_val",
		Choice(
			Alias(
				Sym("_r_blk_seq_r_val"),
				"block_node", true,
			),
			Alias(
				Sym("_r_blk_seq_br_val"),
				"block_node", true,
			),
		))

	g.Define("_r_blk_seq_r_val",
		Alias(
			Sym("_r_blk_seq"),
			"block_sequence", true,
		))

	g.Define("_r_blk_seq_br_val",
		Seq(
			Sym("_r_prp"),
			Alias(
				Sym("_br_blk_seq"),
				"block_sequence", true,
			),
		))

	g.Define("_br_blk_seq_val",
		Choice(
			Alias(
				Sym("_br_blk_seq"),
				"block_sequence", true,
			),
			Seq(
				Sym("_br_prp"),
				Alias(
					Sym("_br_blk_seq"),
					"block_sequence", true,
				),
			),
		))

	g.Define("_r_blk_seq_spc_val",
		Seq(
			Sym("_r_prp"),
			Alias(
				Sym("_b_blk_seq_spc"),
				"block_sequence", true,
			),
		))

	g.Define("_br_blk_seq_spc_val",
		Seq(
			Sym("_br_prp"),
			Alias(
				Sym("_b_blk_seq_spc"),
				"block_sequence", true,
			),
		))

	g.Define("_b_blk_seq_spc_val",
		Alias(
			Sym("_b_blk_seq_spc"),
			"block_sequence", true,
		))

	g.Define("_r_blk_seq",
		Seq(
			Alias(
				Sym("_r_blk_seq_itm"),
				"block_sequence_item", true,
			),
			Repeat(
				Alias(
					Sym("_b_blk_seq_itm"),
					"block_sequence_item", true,
				),
			),
			Sym("_bl"),
		))

	g.Define("_br_blk_seq",
		Seq(
			Alias(
				Sym("_br_blk_seq_itm"),
				"block_sequence_item", true,
			),
			Repeat(
				Alias(
					Sym("_b_blk_seq_itm"),
					"block_sequence_item", true,
				),
			),
			Sym("_bl"),
		))

	g.Define("_b_blk_seq_spc",
		Seq(
			Repeat1(
				Alias(
					Sym("_b_blk_seq_itm"),
					"block_sequence_item", true,
				),
			),
			Sym("_bl"),
		))

	g.Define("_r_blk_seq_itm",
		Seq(
			Alias(
				Sym("_r_blk_seq_bgn"),
				"-", false,
			),
			Choice(
				Sym("_blk_seq_itm_tal"),
				Blank(),
			),
		))

	g.Define("_br_blk_seq_itm",
		Seq(
			Alias(
				Sym("_br_blk_seq_bgn"),
				"-", false,
			),
			Choice(
				Sym("_blk_seq_itm_tal"),
				Blank(),
			),
		))

	g.Define("_b_blk_seq_itm",
		Seq(
			Alias(
				Sym("_b_blk_seq_bgn"),
				"-", false,
			),
			Choice(
				Sym("_blk_seq_itm_tal"),
				Blank(),
			),
		))

	g.Define("_blk_seq_itm_tal",
		Choice(
			Sym("_r_blk_seq_val"),
			Alias(
				Sym("_br_blk_seq_val"),
				"block_node", true,
			),
			Sym("_r_blk_map_val"),
			Alias(
				Sym("_br_blk_map_val"),
				"block_node", true,
			),
			Alias(
				Sym("_r_blk_str_val"),
				"block_node", true,
			),
			Alias(
				Sym("_br_blk_str_val"),
				"block_node", true,
			),
			Sym("_r_flw_val_blk"),
			Sym("_br_flw_val_blk"),
		))

	g.Define("_r_blk_map_val",
		Choice(
			Alias(
				Sym("_r_blk_map_r_val"),
				"block_node", true,
			),
			Alias(
				Sym("_r_blk_map_br_val"),
				"block_node", true,
			),
		))

	g.Define("_r_blk_map_r_val",
		Alias(
			Sym("_r_blk_map"),
			"block_mapping", true,
		))

	g.Define("_r_blk_map_br_val",
		Seq(
			Sym("_r_prp"),
			Alias(
				Sym("_br_blk_map"),
				"block_mapping", true,
			),
		))

	g.Define("_br_blk_map_val",
		Choice(
			Alias(
				Sym("_br_blk_map"),
				"block_mapping", true,
			),
			Seq(
				Sym("_br_prp"),
				Alias(
					Sym("_br_blk_map"),
					"block_mapping", true,
				),
			),
		))

	g.Define("_r_blk_map",
		Seq(
			Sym("_r_blk_map_itm"),
			Repeat(
				Sym("_b_blk_map_itm"),
			),
			Sym("_bl"),
		))

	g.Define("_br_blk_map",
		Seq(
			Sym("_br_blk_map_itm"),
			Repeat(
				Sym("_b_blk_map_itm"),
			),
			Sym("_bl"),
		))

	g.Define("_r_blk_map_itm",
		Choice(
			Alias(
				Sym("_r_blk_exp_itm"),
				"block_mapping_pair", true,
			),
			Alias(
				Sym("_r_blk_imp_itm"),
				"block_mapping_pair", true,
			),
		))

	g.Define("_br_blk_map_itm",
		Choice(
			Alias(
				Sym("_br_blk_exp_itm"),
				"block_mapping_pair", true,
			),
			Alias(
				Sym("_br_blk_imp_itm"),
				"block_mapping_pair", true,
			),
		))

	g.Define("_b_blk_map_itm",
		Choice(
			Alias(
				Sym("_b_blk_exp_itm"),
				"block_mapping_pair", true,
			),
			Alias(
				Sym("_b_blk_imp_itm"),
				"block_mapping_pair", true,
			),
		))

	g.Define("_r_blk_exp_itm",
		PrecRight(0,
			Choice(
				Seq(
					Sym("_r_blk_key_itm"),
					Choice(
						Sym("_b_blk_val_itm"),
						Blank(),
					),
				),
				Sym("_r_blk_val_itm"),
			),
		))

	g.Define("_br_blk_exp_itm",
		PrecRight(0,
			Choice(
				Seq(
					Sym("_br_blk_key_itm"),
					Choice(
						Sym("_b_blk_val_itm"),
						Blank(),
					),
				),
				Sym("_br_blk_val_itm"),
			),
		))

	g.Define("_b_blk_exp_itm",
		PrecRight(0,
			Choice(
				Seq(
					Sym("_b_blk_key_itm"),
					Choice(
						Sym("_b_blk_val_itm"),
						Blank(),
					),
				),
				Sym("_b_blk_val_itm"),
			),
		))

	g.Define("_r_blk_key_itm",
		Seq(
			Alias(
				Sym("_r_blk_key_bgn"),
				"?", false,
			),
			Choice(
				Field("key",
					Sym("_blk_exp_itm_tal"),
				),
				Blank(),
			),
		))

	g.Define("_br_blk_key_itm",
		Seq(
			Alias(
				Sym("_br_blk_key_bgn"),
				"?", false,
			),
			Choice(
				Field("key",
					Sym("_blk_exp_itm_tal"),
				),
				Blank(),
			),
		))

	g.Define("_b_blk_key_itm",
		Seq(
			Alias(
				Sym("_b_blk_key_bgn"),
				"?", false,
			),
			Choice(
				Field("key",
					Sym("_blk_exp_itm_tal"),
				),
				Blank(),
			),
		))

	g.Define("_r_blk_val_itm",
		Seq(
			Alias(
				Sym("_r_blk_val_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_blk_exp_itm_tal"),
				),
				Blank(),
			),
		))

	g.Define("_br_blk_val_itm",
		Seq(
			Alias(
				Sym("_br_blk_val_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_blk_exp_itm_tal"),
				),
				Blank(),
			),
		))

	g.Define("_b_blk_val_itm",
		Seq(
			Alias(
				Sym("_b_blk_val_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_blk_exp_itm_tal"),
				),
				Blank(),
			),
		))

	g.Define("_r_blk_imp_itm",
		Seq(
			Field("key",
				Sym("_r_sgl_flw_val_blk"),
			),
			Sym("_blk_imp_itm_tal"),
		))

	g.Define("_br_blk_imp_itm",
		Seq(
			Field("key",
				Sym("_br_sgl_flw_val_blk"),
			),
			Sym("_blk_imp_itm_tal"),
		))

	g.Define("_b_blk_imp_itm",
		Seq(
			Field("key",
				Sym("_b_sgl_flw_val_blk"),
			),
			Sym("_blk_imp_itm_tal"),
		))

	g.Define("_blk_exp_itm_tal",
		Choice(
			Sym("_blk_seq_itm_tal"),
			Alias(
				Sym("_r_blk_seq_spc_val"),
				"block_node", true,
			),
			Alias(
				Sym("_br_blk_seq_spc_val"),
				"block_node", true,
			),
			Alias(
				Sym("_b_blk_seq_spc_val"),
				"block_node", true,
			),
		))

	g.Define("_blk_imp_itm_tal",
		Seq(
			Alias(
				Sym("_r_blk_imp_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Choice(
						Alias(
							Sym("_r_blk_seq_br_val"),
							"block_node", true,
						),
						Alias(
							Sym("_br_blk_seq_val"),
							"block_node", true,
						),
						Alias(
							Sym("_r_blk_seq_spc_val"),
							"block_node", true,
						),
						Alias(
							Sym("_br_blk_seq_spc_val"),
							"block_node", true,
						),
						Alias(
							Sym("_b_blk_seq_spc_val"),
							"block_node", true,
						),
						Alias(
							Sym("_r_blk_map_br_val"),
							"block_node", true,
						),
						Alias(
							Sym("_br_blk_map_val"),
							"block_node", true,
						),
						Alias(
							Sym("_r_blk_str_val"),
							"block_node", true,
						),
						Alias(
							Sym("_br_blk_str_val"),
							"block_node", true,
						),
						Sym("_r_flw_val_blk"),
						Sym("_br_flw_val_blk"),
					),
				),
				Blank(),
			),
		))

	g.Define("_r_blk_str_val",
		Choice(
			Alias(
				Sym("_r_blk_str"),
				"block_scalar", true,
			),
			Seq(
				Sym("_r_prp"),
				Choice(
					Alias(
						Sym("_r_blk_str"),
						"block_scalar", true,
					),
					Alias(
						Sym("_br_blk_str"),
						"block_scalar", true,
					),
				),
			),
		))

	g.Define("_br_blk_str_val",
		Choice(
			Alias(
				Sym("_br_blk_str"),
				"block_scalar", true,
			),
			Seq(
				Sym("_br_prp"),
				Choice(
					Alias(
						Sym("_r_blk_str"),
						"block_scalar", true,
					),
					Alias(
						Sym("_br_blk_str"),
						"block_scalar", true,
					),
				),
			),
		))

	g.Define("_r_blk_str",
		Seq(
			Choice(
				Alias(
					Sym("_r_blk_lit_bgn"),
					"|", false,
				),
				Alias(
					Sym("_r_blk_fld_bgn"),
					">", false,
				),
			),
			Repeat(
				Sym("_br_blk_str_ctn"),
			),
			Sym("_bl"),
		))

	g.Define("_br_blk_str",
		Seq(
			Choice(
				Alias(
					Sym("_br_blk_lit_bgn"),
					"|", false,
				),
				Alias(
					Sym("_br_blk_fld_bgn"),
					">", false,
				),
			),
			Repeat(
				Sym("_br_blk_str_ctn"),
			),
			Sym("_bl"),
		))

	g.Define("_r_flw_val_blk",
		Choice(
			Sym("_r_flw_jsl_val"),
			Sym("_r_flw_njl_val_blk"),
		))

	g.Define("_br_flw_val_blk",
		Choice(
			Sym("_br_flw_jsl_val"),
			Sym("_br_flw_njl_val_blk"),
		))

	g.Define("_r_sgl_flw_val_blk",
		Choice(
			Sym("_r_sgl_flw_jsl_val"),
			Sym("_r_sgl_flw_njl_val_blk"),
		))

	g.Define("_br_sgl_flw_val_blk",
		Choice(
			Sym("_br_sgl_flw_jsl_val"),
			Sym("_br_sgl_flw_njl_val_blk"),
		))

	g.Define("_b_sgl_flw_val_blk",
		Choice(
			Sym("_b_sgl_flw_jsl_val"),
			Sym("_b_sgl_flw_njl_val_blk"),
		))

	g.Define("_r_flw_val_flw",
		Choice(
			Sym("_r_flw_jsl_val"),
			Sym("_r_flw_njl_val_flw"),
		))

	g.Define("_br_flw_val_flw",
		Choice(
			Sym("_br_flw_jsl_val"),
			Sym("_br_flw_njl_val_flw"),
		))

	g.Define("_r_sgl_flw_val_flw",
		Choice(
			Sym("_r_sgl_flw_jsl_val"),
			Sym("_r_sgl_flw_njl_val_flw"),
		))

	g.Define("_r_flw_jsl_val",
		Choice(
			Alias(
				Sym("_r_flw_seq_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_flw_map_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_dqt_str_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sqt_str_val"),
				"flow_node", true,
			),
		))

	g.Define("_br_flw_jsl_val",
		Choice(
			Alias(
				Sym("_br_flw_seq_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_flw_map_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_dqt_str_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_sqt_str_val"),
				"flow_node", true,
			),
		))

	g.Define("_r_sgl_flw_jsl_val",
		Choice(
			Alias(
				Sym("_r_sgl_flw_seq_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sgl_flw_map_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sgl_dqt_str_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sgl_sqt_str_val"),
				"flow_node", true,
			),
		))

	g.Define("_br_sgl_flw_jsl_val",
		Choice(
			Alias(
				Sym("_br_sgl_flw_seq_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_sgl_flw_map_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_sgl_dqt_str_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_sgl_sqt_str_val"),
				"flow_node", true,
			),
		))

	g.Define("_b_sgl_flw_jsl_val",
		Choice(
			Alias(
				Sym("_b_sgl_flw_seq_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_b_sgl_flw_map_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_b_sgl_dqt_str_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_b_sgl_sqt_str_val"),
				"flow_node", true,
			),
		))

	g.Define("_r_flw_njl_val_blk",
		Choice(
			Alias(
				Sym("_r_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_pln_blk_val"),
				"flow_node", true,
			),
		))

	g.Define("_br_flw_njl_val_blk",
		Choice(
			Alias(
				Sym("_br_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_pln_blk_val"),
				"flow_node", true,
			),
		))

	g.Define("_r_sgl_flw_njl_val_blk",
		Choice(
			Alias(
				Sym("_r_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sgl_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sgl_pln_blk_val"),
				"flow_node", true,
			),
		))

	g.Define("_br_sgl_flw_njl_val_blk",
		Choice(
			Alias(
				Sym("_br_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_sgl_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_sgl_pln_blk_val"),
				"flow_node", true,
			),
		))

	g.Define("_b_sgl_flw_njl_val_blk",
		Choice(
			Alias(
				Sym("_b_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_b_sgl_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_b_sgl_pln_blk_val"),
				"flow_node", true,
			),
		))

	g.Define("_r_flw_njl_val_flw",
		Choice(
			Alias(
				Sym("_r_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_pln_flw_val"),
				"flow_node", true,
			),
		))

	g.Define("_br_flw_njl_val_flw",
		Choice(
			Alias(
				Sym("_br_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_br_pln_flw_val"),
				"flow_node", true,
			),
		))

	g.Define("_r_sgl_flw_njl_val_flw",
		Choice(
			Alias(
				Sym("_r_als_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sgl_prp_val"),
				"flow_node", true,
			),
			Alias(
				Sym("_r_sgl_pln_flw_val"),
				"flow_node", true,
			),
		))

	g.Define("_r_flw_seq_val",
		Choice(
			Alias(
				Sym("_r_flw_seq"),
				"flow_sequence", true,
			),
			Seq(
				Sym("_r_prp"),
				Choice(
					Alias(
						Sym("_r_flw_seq"),
						"flow_sequence", true,
					),
					Alias(
						Sym("_br_flw_seq"),
						"flow_sequence", true,
					),
				),
			),
		))

	g.Define("_br_flw_seq_val",
		Choice(
			Alias(
				Sym("_br_flw_seq"),
				"flow_sequence", true,
			),
			Seq(
				Sym("_br_prp"),
				Choice(
					Alias(
						Sym("_r_flw_seq"),
						"flow_sequence", true,
					),
					Alias(
						Sym("_br_flw_seq"),
						"flow_sequence", true,
					),
				),
			),
		))

	g.Define("_r_sgl_flw_seq_val",
		Choice(
			Alias(
				Sym("_r_sgl_flw_seq"),
				"flow_sequence", true,
			),
			Seq(
				Sym("_r_sgl_prp"),
				Alias(
					Sym("_r_sgl_flw_seq"),
					"flow_sequence", true,
				),
			),
		))

	g.Define("_br_sgl_flw_seq_val",
		Choice(
			Alias(
				Sym("_br_sgl_flw_seq"),
				"flow_sequence", true,
			),
			Seq(
				Sym("_br_sgl_prp"),
				Alias(
					Sym("_r_sgl_flw_seq"),
					"flow_sequence", true,
				),
			),
		))

	g.Define("_b_sgl_flw_seq_val",
		Choice(
			Alias(
				Sym("_b_sgl_flw_seq"),
				"flow_sequence", true,
			),
			Seq(
				Sym("_b_sgl_prp"),
				Alias(
					Sym("_r_sgl_flw_seq"),
					"flow_sequence", true,
				),
			),
		))

	g.Define("_r_flw_seq",
		Seq(
			Alias(
				Sym("_r_flw_seq_bgn"),
				"[", false,
			),
			Sym("_flw_seq_tal"),
		))

	g.Define("_br_flw_seq",
		Seq(
			Alias(
				Sym("_br_flw_seq_bgn"),
				"[", false,
			),
			Sym("_flw_seq_tal"),
		))

	g.Define("_r_sgl_flw_seq",
		Seq(
			Alias(
				Sym("_r_flw_seq_bgn"),
				"[", false,
			),
			Sym("_sgl_flw_seq_tal"),
		))

	g.Define("_br_sgl_flw_seq",
		Seq(
			Alias(
				Sym("_br_flw_seq_bgn"),
				"[", false,
			),
			Sym("_sgl_flw_seq_tal"),
		))

	g.Define("_b_sgl_flw_seq",
		Seq(
			Alias(
				Sym("_b_flw_seq_bgn"),
				"[", false,
			),
			Sym("_sgl_flw_seq_tal"),
		))

	g.Define("_flw_seq_tal",
		Seq(
			Choice(
				Choice(
					Sym("_r_flw_seq_dat"),
					Sym("_br_flw_seq_dat"),
				),
				Blank(),
			),
			Choice(
				Alias(
					Sym("_r_flw_seq_end"),
					"]", false,
				),
				Alias(
					Sym("_br_flw_seq_end"),
					"]", false,
				),
				Alias(
					Sym("_b_flw_seq_end"),
					"]", false,
				),
			),
		))

	g.Define("_sgl_flw_seq_tal",
		Seq(
			Choice(
				Sym("_r_sgl_flw_col_dat"),
				Blank(),
			),
			Alias(
				Sym("_r_flw_seq_end"),
				"]", false,
			),
		))

	g.Define("_r_flw_map_val",
		Choice(
			Alias(
				Sym("_r_flw_map"),
				"flow_mapping", true,
			),
			Seq(
				Sym("_r_prp"),
				Choice(
					Alias(
						Sym("_r_flw_map"),
						"flow_mapping", true,
					),
					Alias(
						Sym("_br_flw_map"),
						"flow_mapping", true,
					),
				),
			),
		))

	g.Define("_br_flw_map_val",
		Choice(
			Alias(
				Sym("_br_flw_map"),
				"flow_mapping", true,
			),
			Seq(
				Sym("_br_prp"),
				Choice(
					Alias(
						Sym("_r_flw_map"),
						"flow_mapping", true,
					),
					Alias(
						Sym("_br_flw_map"),
						"flow_mapping", true,
					),
				),
			),
		))

	g.Define("_r_sgl_flw_map_val",
		Choice(
			Alias(
				Sym("_r_sgl_flw_map"),
				"flow_mapping", true,
			),
			Seq(
				Sym("_r_sgl_prp"),
				Alias(
					Sym("_r_sgl_flw_map"),
					"flow_mapping", true,
				),
			),
		))

	g.Define("_br_sgl_flw_map_val",
		Choice(
			Alias(
				Sym("_br_sgl_flw_map"),
				"flow_mapping", true,
			),
			Seq(
				Sym("_br_sgl_prp"),
				Alias(
					Sym("_r_sgl_flw_map"),
					"flow_mapping", true,
				),
			),
		))

	g.Define("_b_sgl_flw_map_val",
		Choice(
			Alias(
				Sym("_b_sgl_flw_map"),
				"flow_mapping", true,
			),
			Seq(
				Sym("_b_sgl_prp"),
				Alias(
					Sym("_r_sgl_flw_map"),
					"flow_mapping", true,
				),
			),
		))

	g.Define("_r_flw_map",
		Seq(
			Alias(
				Sym("_r_flw_map_bgn"),
				"{", false,
			),
			Sym("_flw_map_tal"),
		))

	g.Define("_br_flw_map",
		Seq(
			Alias(
				Sym("_br_flw_map_bgn"),
				"{", false,
			),
			Sym("_flw_map_tal"),
		))

	g.Define("_r_sgl_flw_map",
		Seq(
			Alias(
				Sym("_r_flw_map_bgn"),
				"{", false,
			),
			Sym("_sgl_flw_map_tal"),
		))

	g.Define("_br_sgl_flw_map",
		Seq(
			Alias(
				Sym("_br_flw_map_bgn"),
				"{", false,
			),
			Sym("_sgl_flw_map_tal"),
		))

	g.Define("_b_sgl_flw_map",
		Seq(
			Alias(
				Sym("_b_flw_map_bgn"),
				"{", false,
			),
			Sym("_sgl_flw_map_tal"),
		))

	g.Define("_flw_map_tal",
		Seq(
			Choice(
				Choice(
					Sym("_r_flw_map_dat"),
					Sym("_br_flw_map_dat"),
				),
				Blank(),
			),
			Choice(
				Alias(
					Sym("_r_flw_map_end"),
					"}", false,
				),
				Alias(
					Sym("_br_flw_map_end"),
					"}", false,
				),
				Alias(
					Sym("_b_flw_map_end"),
					"}", false,
				),
			),
		))

	g.Define("_sgl_flw_map_tal",
		Seq(
			Choice(
				Sym("_r_sgl_flw_col_dat"),
				Blank(),
			),
			Alias(
				Sym("_r_flw_map_end"),
				"}", false,
			),
		))

	g.Define("_r_flw_seq_dat",
		Seq(
			Sym("_r_flw_seq_itm"),
			Repeat(
				Sym("_flw_seq_dat_rpt"),
			),
			Choice(
				Choice(
					Alias(
						Sym("_r_flw_sep_bgn"),
						",", false,
					),
					Alias(
						Sym("_br_flw_sep_bgn"),
						",", false,
					),
				),
				Blank(),
			),
		))

	g.Define("_br_flw_seq_dat",
		Seq(
			Sym("_br_flw_seq_itm"),
			Repeat(
				Sym("_flw_seq_dat_rpt"),
			),
			Choice(
				Choice(
					Alias(
						Sym("_r_flw_sep_bgn"),
						",", false,
					),
					Alias(
						Sym("_br_flw_sep_bgn"),
						",", false,
					),
				),
				Blank(),
			),
		))

	g.Define("_r_flw_map_dat",
		Seq(
			Sym("_r_flw_map_itm"),
			Repeat(
				Sym("_flw_map_dat_rpt"),
			),
			Choice(
				Choice(
					Alias(
						Sym("_r_flw_sep_bgn"),
						",", false,
					),
					Alias(
						Sym("_br_flw_sep_bgn"),
						",", false,
					),
				),
				Blank(),
			),
		))

	g.Define("_br_flw_map_dat",
		Seq(
			Sym("_br_flw_map_itm"),
			Repeat(
				Sym("_flw_map_dat_rpt"),
			),
			Choice(
				Choice(
					Alias(
						Sym("_r_flw_sep_bgn"),
						",", false,
					),
					Alias(
						Sym("_br_flw_sep_bgn"),
						",", false,
					),
				),
				Blank(),
			),
		))

	g.Define("_r_sgl_flw_col_dat",
		Seq(
			Sym("_r_sgl_flw_col_itm"),
			Repeat(
				Sym("_sgl_flw_col_dat_rpt"),
			),
			Choice(
				Alias(
					Sym("_r_flw_sep_bgn"),
					",", false,
				),
				Blank(),
			),
		))

	g.Define("_flw_seq_dat_rpt",
		Seq(
			Choice(
				Alias(
					Sym("_r_flw_sep_bgn"),
					",", false,
				),
				Alias(
					Sym("_br_flw_sep_bgn"),
					",", false,
				),
			),
			Choice(
				Sym("_r_flw_seq_itm"),
				Sym("_br_flw_seq_itm"),
			),
		))

	g.Define("_flw_map_dat_rpt",
		Seq(
			Choice(
				Alias(
					Sym("_r_flw_sep_bgn"),
					",", false,
				),
				Alias(
					Sym("_br_flw_sep_bgn"),
					",", false,
				),
			),
			Choice(
				Sym("_r_flw_map_itm"),
				Sym("_br_flw_map_itm"),
			),
		))

	g.Define("_sgl_flw_col_dat_rpt",
		Seq(
			Alias(
				Sym("_r_flw_sep_bgn"),
				",", false,
			),
			Sym("_r_sgl_flw_col_itm"),
		))

	g.Define("_r_flw_seq_itm",
		Choice(
			Sym("_r_flw_val_flw"),
			Alias(
				Sym("_r_flw_exp_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_r_flw_imp_r_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_r_flw_njl_ann_par"),
				"flow_pair", true,
			),
		))

	g.Define("_br_flw_seq_itm",
		Choice(
			Sym("_br_flw_val_flw"),
			Alias(
				Sym("_br_flw_exp_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_br_flw_imp_r_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_br_flw_njl_ann_par"),
				"flow_pair", true,
			),
		))

	g.Define("_r_flw_map_itm",
		Choice(
			Sym("_r_flw_val_flw"),
			Alias(
				Sym("_r_flw_exp_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_r_flw_imp_r_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_r_flw_imp_br_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_r_flw_njl_ann_par"),
				"flow_pair", true,
			),
		))

	g.Define("_br_flw_map_itm",
		Choice(
			Sym("_br_flw_val_flw"),
			Alias(
				Sym("_br_flw_exp_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_br_flw_imp_r_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_br_flw_imp_br_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_br_flw_njl_ann_par"),
				"flow_pair", true,
			),
		))

	g.Define("_r_sgl_flw_col_itm",
		Choice(
			Sym("_r_sgl_flw_val_flw"),
			Alias(
				Sym("_r_sgl_flw_exp_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_r_sgl_flw_imp_par"),
				"flow_pair", true,
			),
			Alias(
				Sym("_r_sgl_flw_njl_ann_par"),
				"flow_pair", true,
			),
		))

	g.Define("_r_flw_exp_par",
		Seq(
			Alias(
				Sym("_r_flw_key_bgn"),
				"?", false,
			),
			Choice(
				Choice(
					Sym("_r_flw_imp_r_par"),
					Sym("_r_flw_imp_br_par"),
					Sym("_br_flw_imp_r_par"),
					Sym("_br_flw_imp_br_par"),
				),
				Blank(),
			),
		))

	g.Define("_br_flw_exp_par",
		Seq(
			Alias(
				Sym("_br_flw_key_bgn"),
				"?", false,
			),
			Choice(
				Choice(
					Sym("_r_flw_imp_r_par"),
					Sym("_r_flw_imp_br_par"),
					Sym("_br_flw_imp_r_par"),
					Sym("_br_flw_imp_br_par"),
				),
				Blank(),
			),
		))

	g.Define("_r_sgl_flw_exp_par",
		Seq(
			Alias(
				Sym("_r_flw_key_bgn"),
				"?", false,
			),
			Choice(
				Sym("_r_sgl_flw_imp_par"),
				Blank(),
			),
		))

	g.Define("_r_flw_imp_r_par",
		Choice(
			Seq(
				Field("key",
					Sym("_r_flw_jsl_val"),
				),
				Sym("_r_flw_jsl_ann_par"),
			),
			Seq(
				Field("key",
					Sym("_r_flw_njl_val_flw"),
				),
				Sym("_r_flw_njl_ann_par"),
			),
		))

	g.Define("_r_flw_imp_br_par",
		Choice(
			Seq(
				Field("key",
					Sym("_r_flw_jsl_val"),
				),
				Sym("_br_flw_jsl_ann_par"),
			),
			Seq(
				Field("key",
					Sym("_r_flw_njl_val_flw"),
				),
				Sym("_br_flw_njl_ann_par"),
			),
		))

	g.Define("_br_flw_imp_r_par",
		Choice(
			Seq(
				Field("key",
					Sym("_br_flw_jsl_val"),
				),
				Sym("_r_flw_jsl_ann_par"),
			),
			Seq(
				Field("key",
					Sym("_br_flw_njl_val_flw"),
				),
				Sym("_r_flw_njl_ann_par"),
			),
		))

	g.Define("_br_flw_imp_br_par",
		Choice(
			Seq(
				Field("key",
					Sym("_br_flw_jsl_val"),
				),
				Sym("_br_flw_jsl_ann_par"),
			),
			Seq(
				Field("key",
					Sym("_br_flw_njl_val_flw"),
				),
				Sym("_br_flw_njl_ann_par"),
			),
		))

	g.Define("_r_sgl_flw_imp_par",
		Choice(
			Seq(
				Field("key",
					Sym("_r_sgl_flw_jsl_val"),
				),
				Sym("_r_sgl_flw_jsl_ann_par"),
			),
			Seq(
				Field("key",
					Sym("_r_sgl_flw_njl_val_flw"),
				),
				Sym("_r_sgl_flw_njl_ann_par"),
			),
		))

	g.Define("_r_flw_jsl_ann_par",
		Seq(
			Alias(
				Sym("_r_flw_jsv_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_flw_ann_par_tal"),
				),
				Blank(),
			),
		))

	g.Define("_br_flw_jsl_ann_par",
		Seq(
			Alias(
				Sym("_br_flw_jsv_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_flw_ann_par_tal"),
				),
				Blank(),
			),
		))

	g.Define("_r_sgl_flw_jsl_ann_par",
		Seq(
			Alias(
				Sym("_r_flw_jsv_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_sgl_flw_ann_par_tal"),
				),
				Blank(),
			),
		))

	g.Define("_r_flw_njl_ann_par",
		Seq(
			Alias(
				Sym("_r_flw_njv_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_flw_ann_par_tal"),
				),
				Blank(),
			),
		))

	g.Define("_br_flw_njl_ann_par",
		Seq(
			Alias(
				Sym("_br_flw_njv_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_flw_ann_par_tal"),
				),
				Blank(),
			),
		))

	g.Define("_r_sgl_flw_njl_ann_par",
		Seq(
			Alias(
				Sym("_r_flw_njv_bgn"),
				":", false,
			),
			Choice(
				Field("value",
					Sym("_sgl_flw_ann_par_tal"),
				),
				Blank(),
			),
		))

	g.Define("_flw_ann_par_tal",
		Choice(
			Sym("_r_flw_val_flw"),
			Sym("_br_flw_val_flw"),
		))

	g.Define("_sgl_flw_ann_par_tal",
		Sym("_r_sgl_flw_val_flw"))

	g.Define("_r_dqt_str_val",
		Choice(
			Alias(
				Sym("_r_dqt_str"),
				"double_quote_scalar", true,
			),
			Seq(
				Sym("_r_prp"),
				Choice(
					Alias(
						Sym("_r_dqt_str"),
						"double_quote_scalar", true,
					),
					Alias(
						Sym("_br_dqt_str"),
						"double_quote_scalar", true,
					),
				),
			),
		))

	g.Define("_br_dqt_str_val",
		Choice(
			Alias(
				Sym("_br_dqt_str"),
				"double_quote_scalar", true,
			),
			Seq(
				Sym("_br_prp"),
				Choice(
					Alias(
						Sym("_r_dqt_str"),
						"double_quote_scalar", true,
					),
					Alias(
						Sym("_br_dqt_str"),
						"double_quote_scalar", true,
					),
				),
			),
		))

	g.Define("_r_sgl_dqt_str_val",
		Choice(
			Alias(
				Sym("_r_sgl_dqt_str"),
				"double_quote_scalar", true,
			),
			Seq(
				Sym("_r_sgl_prp"),
				Alias(
					Sym("_r_sgl_dqt_str"),
					"double_quote_scalar", true,
				),
			),
		))

	g.Define("_br_sgl_dqt_str_val",
		Choice(
			Alias(
				Sym("_br_sgl_dqt_str"),
				"double_quote_scalar", true,
			),
			Seq(
				Sym("_br_sgl_prp"),
				Alias(
					Sym("_r_sgl_dqt_str"),
					"double_quote_scalar", true,
				),
			),
		))

	g.Define("_b_sgl_dqt_str_val",
		Choice(
			Alias(
				Sym("_b_sgl_dqt_str"),
				"double_quote_scalar", true,
			),
			Seq(
				Sym("_b_sgl_prp"),
				Alias(
					Sym("_r_sgl_dqt_str"),
					"double_quote_scalar", true,
				),
			),
		))

	g.Define("_r_dqt_str",
		Seq(
			Alias(
				Sym("_r_dqt_str_bgn"),
				"\"", false,
			),
			Choice(
				Sym("_r_sgl_dqt_ctn"),
				Blank(),
			),
			Choice(
				Alias(
					Sym("_r_dqt_esc_nwl"),
					"escape_sequence", true,
				),
				Blank(),
			),
			Repeat(
				Sym("_br_mtl_dqt_ctn"),
			),
			Choice(
				Alias(
					Sym("_r_dqt_str_end"),
					"\"", false,
				),
				Alias(
					Sym("_br_dqt_str_end"),
					"\"", false,
				),
			),
		))

	g.Define("_br_dqt_str",
		Seq(
			Alias(
				Sym("_br_dqt_str_bgn"),
				"\"", false,
			),
			Choice(
				Sym("_r_sgl_dqt_ctn"),
				Blank(),
			),
			Choice(
				Alias(
					Sym("_r_dqt_esc_nwl"),
					"escape_sequence", true,
				),
				Blank(),
			),
			Repeat(
				Sym("_br_mtl_dqt_ctn"),
			),
			Choice(
				Alias(
					Sym("_r_dqt_str_end"),
					"\"", false,
				),
				Alias(
					Sym("_br_dqt_str_end"),
					"\"", false,
				),
			),
		))

	g.Define("_r_sgl_dqt_str",
		Seq(
			Alias(
				Sym("_r_dqt_str_bgn"),
				"\"", false,
			),
			Choice(
				Sym("_r_sgl_dqt_ctn"),
				Blank(),
			),
			Alias(
				Sym("_r_dqt_str_end"),
				"\"", false,
			),
		))

	g.Define("_br_sgl_dqt_str",
		Seq(
			Alias(
				Sym("_br_dqt_str_bgn"),
				"\"", false,
			),
			Choice(
				Sym("_r_sgl_dqt_ctn"),
				Blank(),
			),
			Alias(
				Sym("_r_dqt_str_end"),
				"\"", false,
			),
		))

	g.Define("_b_sgl_dqt_str",
		Seq(
			Alias(
				Sym("_b_dqt_str_bgn"),
				"\"", false,
			),
			Choice(
				Sym("_r_sgl_dqt_ctn"),
				Blank(),
			),
			Alias(
				Sym("_r_dqt_str_end"),
				"\"", false,
			),
		))

	g.Define("_r_sgl_dqt_ctn",
		Repeat1(
			Choice(
				Sym("_r_dqt_str_ctn"),
				Alias(
					Sym("_r_dqt_esc_seq"),
					"escape_sequence", true,
				),
			),
		))

	g.Define("_br_mtl_dqt_ctn",
		Choice(
			Alias(
				Sym("_br_dqt_esc_nwl"),
				"escape_sequence", true,
			),
			Seq(
				Choice(
					Sym("_br_dqt_str_ctn"),
					Alias(
						Sym("_br_dqt_esc_seq"),
						"escape_sequence", true,
					),
				),
				Repeat(
					Choice(
						Sym("_r_dqt_str_ctn"),
						Alias(
							Sym("_r_dqt_esc_seq"),
							"escape_sequence", true,
						),
					),
				),
				Choice(
					Alias(
						Sym("_r_dqt_esc_nwl"),
						"escape_sequence", true,
					),
					Blank(),
				),
			),
		))

	g.Define("_r_sqt_str_val",
		Choice(
			Alias(
				Sym("_r_sqt_str"),
				"single_quote_scalar", true,
			),
			Seq(
				Sym("_r_prp"),
				Choice(
					Alias(
						Sym("_r_sqt_str"),
						"single_quote_scalar", true,
					),
					Alias(
						Sym("_br_sqt_str"),
						"single_quote_scalar", true,
					),
				),
			),
		))

	g.Define("_br_sqt_str_val",
		Choice(
			Alias(
				Sym("_br_sqt_str"),
				"single_quote_scalar", true,
			),
			Seq(
				Sym("_br_prp"),
				Choice(
					Alias(
						Sym("_r_sqt_str"),
						"single_quote_scalar", true,
					),
					Alias(
						Sym("_br_sqt_str"),
						"single_quote_scalar", true,
					),
				),
			),
		))

	g.Define("_r_sgl_sqt_str_val",
		Choice(
			Alias(
				Sym("_r_sgl_sqt_str"),
				"single_quote_scalar", true,
			),
			Seq(
				Sym("_r_sgl_prp"),
				Alias(
					Sym("_r_sgl_sqt_str"),
					"single_quote_scalar", true,
				),
			),
		))

	g.Define("_br_sgl_sqt_str_val",
		Choice(
			Alias(
				Sym("_br_sgl_sqt_str"),
				"single_quote_scalar", true,
			),
			Seq(
				Sym("_br_sgl_prp"),
				Alias(
					Sym("_r_sgl_sqt_str"),
					"single_quote_scalar", true,
				),
			),
		))

	g.Define("_b_sgl_sqt_str_val",
		Choice(
			Alias(
				Sym("_b_sgl_sqt_str"),
				"single_quote_scalar", true,
			),
			Seq(
				Sym("_b_sgl_prp"),
				Alias(
					Sym("_r_sgl_sqt_str"),
					"single_quote_scalar", true,
				),
			),
		))

	g.Define("_r_sqt_str",
		Seq(
			Alias(
				Sym("_r_sqt_str_bgn"),
				"'", false,
			),
			Choice(
				Sym("_r_sgl_sqt_ctn"),
				Blank(),
			),
			Repeat(
				Sym("_br_mtl_sqt_ctn"),
			),
			Choice(
				Alias(
					Sym("_r_sqt_str_end"),
					"'", false,
				),
				Alias(
					Sym("_br_sqt_str_end"),
					"'", false,
				),
			),
		))

	g.Define("_br_sqt_str",
		Seq(
			Alias(
				Sym("_br_sqt_str_bgn"),
				"'", false,
			),
			Choice(
				Sym("_r_sgl_sqt_ctn"),
				Blank(),
			),
			Repeat(
				Sym("_br_mtl_sqt_ctn"),
			),
			Choice(
				Alias(
					Sym("_r_sqt_str_end"),
					"'", false,
				),
				Alias(
					Sym("_br_sqt_str_end"),
					"'", false,
				),
			),
		))

	g.Define("_r_sgl_sqt_str",
		Seq(
			Alias(
				Sym("_r_sqt_str_bgn"),
				"'", false,
			),
			Choice(
				Sym("_r_sgl_sqt_ctn"),
				Blank(),
			),
			Alias(
				Sym("_r_sqt_str_end"),
				"'", false,
			),
		))

	g.Define("_br_sgl_sqt_str",
		Seq(
			Alias(
				Sym("_br_sqt_str_bgn"),
				"'", false,
			),
			Choice(
				Sym("_r_sgl_sqt_ctn"),
				Blank(),
			),
			Alias(
				Sym("_r_sqt_str_end"),
				"'", false,
			),
		))

	g.Define("_b_sgl_sqt_str",
		Seq(
			Alias(
				Sym("_b_sqt_str_bgn"),
				"'", false,
			),
			Choice(
				Sym("_r_sgl_sqt_ctn"),
				Blank(),
			),
			Alias(
				Sym("_r_sqt_str_end"),
				"'", false,
			),
		))

	g.Define("_r_sgl_sqt_ctn",
		Repeat1(
			Choice(
				Sym("_r_sqt_str_ctn"),
				Alias(
					Sym("_r_sqt_esc_sqt"),
					"escape_sequence", true,
				),
			),
		))

	g.Define("_br_mtl_sqt_ctn",
		Seq(
			Choice(
				Sym("_br_sqt_str_ctn"),
				Alias(
					Sym("_br_sqt_esc_sqt"),
					"escape_sequence", true,
				),
			),
			Repeat(
				Choice(
					Sym("_r_sqt_str_ctn"),
					Alias(
						Sym("_r_sqt_esc_sqt"),
						"escape_sequence", true,
					),
				),
			),
		))

	g.Define("_r_pln_blk_val",
		Choice(
			Sym("_r_pln_blk"),
			Seq(
				Sym("_r_prp"),
				Choice(
					Sym("_r_pln_blk"),
					Sym("_br_pln_blk"),
				),
			),
		))

	g.Define("_br_pln_blk_val",
		Choice(
			Sym("_br_pln_blk"),
			Seq(
				Sym("_br_prp"),
				Choice(
					Sym("_r_pln_blk"),
					Sym("_br_pln_blk"),
				),
			),
		))

	g.Define("_r_sgl_pln_blk_val",
		Choice(
			Alias(
				Sym("_r_sgl_pln_blk"),
				"plain_scalar", true,
			),
			Seq(
				Sym("_r_sgl_prp"),
				Alias(
					Sym("_r_sgl_pln_blk"),
					"plain_scalar", true,
				),
			),
		))

	g.Define("_br_sgl_pln_blk_val",
		Choice(
			Alias(
				Sym("_br_sgl_pln_blk"),
				"plain_scalar", true,
			),
			Seq(
				Sym("_br_sgl_prp"),
				Alias(
					Sym("_r_sgl_pln_blk"),
					"plain_scalar", true,
				),
			),
		))

	g.Define("_b_sgl_pln_blk_val",
		Choice(
			Alias(
				Sym("_b_sgl_pln_blk"),
				"plain_scalar", true,
			),
			Seq(
				Sym("_b_sgl_prp"),
				Alias(
					Sym("_r_sgl_pln_blk"),
					"plain_scalar", true,
				),
			),
		))

	g.Define("_r_pln_blk",
		Choice(
			Alias(
				Sym("_r_sgl_pln_blk"),
				"plain_scalar", true,
			),
			Alias(
				Sym("_r_mtl_pln_blk"),
				"plain_scalar", true,
			),
		))

	g.Define("_br_pln_blk",
		Choice(
			Alias(
				Sym("_br_sgl_pln_blk"),
				"plain_scalar", true,
			),
			Alias(
				Sym("_br_mtl_pln_blk"),
				"plain_scalar", true,
			),
		))

	g.Define("_r_pln_flw_val",
		Choice(
			Sym("_r_pln_flw"),
			Seq(
				Sym("_r_prp"),
				Choice(
					Sym("_r_pln_flw"),
					Sym("_br_pln_flw"),
				),
			),
		))

	g.Define("_br_pln_flw_val",
		Choice(
			Sym("_br_pln_flw"),
			Seq(
				Sym("_br_prp"),
				Choice(
					Sym("_r_pln_flw"),
					Sym("_br_pln_flw"),
				),
			),
		))

	g.Define("_r_sgl_pln_flw_val",
		Choice(
			Alias(
				Sym("_r_sgl_pln_flw"),
				"plain_scalar", true,
			),
			Seq(
				Sym("_r_sgl_prp"),
				Alias(
					Sym("_r_sgl_pln_flw"),
					"plain_scalar", true,
				),
			),
		))

	g.Define("_r_pln_flw",
		Choice(
			Alias(
				Sym("_r_sgl_pln_flw"),
				"plain_scalar", true,
			),
			Alias(
				Sym("_r_mtl_pln_flw"),
				"plain_scalar", true,
			),
		))

	g.Define("_br_pln_flw",
		Choice(
			Alias(
				Sym("_br_sgl_pln_flw"),
				"plain_scalar", true,
			),
			Alias(
				Sym("_br_mtl_pln_flw"),
				"plain_scalar", true,
			),
		))

	g.Define("_r_sgl_pln_blk",
		Choice(
			Alias(
				Sym("_r_sgl_pln_nul_blk"),
				"null_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_bol_blk"),
				"boolean_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_int_blk"),
				"integer_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_flt_blk"),
				"float_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_tms_blk"),
				"timestamp_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_str_blk"),
				"string_scalar", true,
			),
		))

	g.Define("_br_sgl_pln_blk",
		Choice(
			Alias(
				Sym("_br_sgl_pln_nul_blk"),
				"null_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_bol_blk"),
				"boolean_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_int_blk"),
				"integer_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_flt_blk"),
				"float_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_tms_blk"),
				"timestamp_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_str_blk"),
				"string_scalar", true,
			),
		))

	g.Define("_b_sgl_pln_blk",
		Choice(
			Alias(
				Sym("_b_sgl_pln_nul_blk"),
				"null_scalar", true,
			),
			Alias(
				Sym("_b_sgl_pln_bol_blk"),
				"boolean_scalar", true,
			),
			Alias(
				Sym("_b_sgl_pln_int_blk"),
				"integer_scalar", true,
			),
			Alias(
				Sym("_b_sgl_pln_flt_blk"),
				"float_scalar", true,
			),
			Alias(
				Sym("_b_sgl_pln_tms_blk"),
				"timestamp_scalar", true,
			),
			Alias(
				Sym("_b_sgl_pln_str_blk"),
				"string_scalar", true,
			),
		))

	g.Define("_r_sgl_pln_flw",
		Choice(
			Alias(
				Sym("_r_sgl_pln_nul_flw"),
				"null_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_bol_flw"),
				"boolean_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_int_flw"),
				"integer_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_flt_flw"),
				"float_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_tms_flw"),
				"timestamp_scalar", true,
			),
			Alias(
				Sym("_r_sgl_pln_str_flw"),
				"string_scalar", true,
			),
		))

	g.Define("_br_sgl_pln_flw",
		Choice(
			Alias(
				Sym("_br_sgl_pln_nul_flw"),
				"null_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_bol_flw"),
				"boolean_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_int_flw"),
				"integer_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_flt_flw"),
				"float_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_tms_flw"),
				"timestamp_scalar", true,
			),
			Alias(
				Sym("_br_sgl_pln_str_flw"),
				"string_scalar", true,
			),
		))

	g.Define("_r_mtl_pln_blk",
		Alias(
			Sym("_r_mtl_pln_str_blk"),
			"string_scalar", true,
		))

	g.Define("_br_mtl_pln_blk",
		Alias(
			Sym("_br_mtl_pln_str_blk"),
			"string_scalar", true,
		))

	g.Define("_r_mtl_pln_flw",
		Alias(
			Sym("_r_mtl_pln_str_flw"),
			"string_scalar", true,
		))

	g.Define("_br_mtl_pln_flw",
		Alias(
			Sym("_br_mtl_pln_str_flw"),
			"string_scalar", true,
		))

	g.Define("_r_als_val",
		Alias(
			Sym("_r_als"),
			"alias", true,
		))

	g.Define("_br_als_val",
		Alias(
			Sym("_br_als"),
			"alias", true,
		))

	g.Define("_b_als_val",
		Alias(
			Sym("_b_als"),
			"alias", true,
		))

	g.Define("_r_als",
		Seq(
			Alias(
				Sym("_r_als_bgn"),
				"*", false,
			),
			Alias(
				Sym("_r_als_ctn"),
				"alias_name", true,
			),
		))

	g.Define("_br_als",
		Seq(
			Alias(
				Sym("_br_als_bgn"),
				"*", false,
			),
			Alias(
				Sym("_r_als_ctn"),
				"alias_name", true,
			),
		))

	g.Define("_b_als",
		Seq(
			Alias(
				Sym("_b_als_bgn"),
				"*", false,
			),
			Alias(
				Sym("_r_als_ctn"),
				"alias_name", true,
			),
		))

	g.Define("_r_acr",
		Seq(
			Alias(
				Sym("_r_acr_bgn"),
				"&", false,
			),
			Alias(
				Sym("_r_acr_ctn"),
				"anchor_name", true,
			),
		))

	g.Define("_br_acr",
		Seq(
			Alias(
				Sym("_br_acr_bgn"),
				"&", false,
			),
			Alias(
				Sym("_r_acr_ctn"),
				"anchor_name", true,
			),
		))

	g.Define("_b_acr",
		Seq(
			Alias(
				Sym("_b_acr_bgn"),
				"&", false,
			),
			Alias(
				Sym("_r_acr_ctn"),
				"anchor_name", true,
			),
		))

	g.SetExtras(
		Sym("comment"),
	)

	g.SetConflicts(
		[]string{"_r_prp", "_r_sgl_prp"},
		[]string{"_br_prp", "_br_sgl_prp"},
		[]string{"_flw_seq_tal", "_sgl_flw_seq_tal"},
		[]string{"_flw_map_tal", "_sgl_flw_map_tal"},
		[]string{"_flw_ann_par_tal", "_sgl_flw_ann_par_tal"},
		[]string{"_r_flw_seq_itm", "_r_sgl_flw_col_itm"},
		[]string{"_r_flw_map_itm", "_r_sgl_flw_col_itm"},
		[]string{"_r_flw_njl_ann_par", "_r_sgl_flw_njl_ann_par"},
		[]string{"_r_flw_exp_par", "_r_sgl_flw_exp_par"},
		[]string{"_r_dqt_str", "_r_sgl_dqt_str"},
		[]string{"_r_sqt_str", "_r_sgl_sqt_str"},
		[]string{"_r_pln_flw_val", "_r_sgl_pln_flw_val"},
		[]string{"_r_prp"},
		[]string{"_br_prp"},
	)

	g.SetExternals(
		Sym("_eof"),
		Sym("_s_dir_yml_bgn"),
		Sym("_r_dir_yml_ver"),
		Sym("_s_dir_tag_bgn"),
		Sym("_r_dir_tag_hdl"),
		Sym("_r_dir_tag_pfx"),
		Sym("_s_dir_rsv_bgn"),
		Sym("_r_dir_rsv_prm"),
		Sym("_s_drs_end"),
		Sym("_s_doc_end"),
		Sym("_r_blk_seq_bgn"),
		Sym("_br_blk_seq_bgn"),
		Sym("_b_blk_seq_bgn"),
		Sym("_r_blk_key_bgn"),
		Sym("_br_blk_key_bgn"),
		Sym("_b_blk_key_bgn"),
		Sym("_r_blk_val_bgn"),
		Sym("_br_blk_val_bgn"),
		Sym("_b_blk_val_bgn"),
		Sym("_r_blk_imp_bgn"),
		Sym("_r_blk_lit_bgn"),
		Sym("_br_blk_lit_bgn"),
		Sym("_r_blk_fld_bgn"),
		Sym("_br_blk_fld_bgn"),
		Sym("_br_blk_str_ctn"),
		Sym("_r_flw_seq_bgn"),
		Sym("_br_flw_seq_bgn"),
		Sym("_b_flw_seq_bgn"),
		Sym("_r_flw_seq_end"),
		Sym("_br_flw_seq_end"),
		Sym("_b_flw_seq_end"),
		Sym("_r_flw_map_bgn"),
		Sym("_br_flw_map_bgn"),
		Sym("_b_flw_map_bgn"),
		Sym("_r_flw_map_end"),
		Sym("_br_flw_map_end"),
		Sym("_b_flw_map_end"),
		Sym("_r_flw_sep_bgn"),
		Sym("_br_flw_sep_bgn"),
		Sym("_r_flw_key_bgn"),
		Sym("_br_flw_key_bgn"),
		Sym("_r_flw_jsv_bgn"),
		Sym("_br_flw_jsv_bgn"),
		Sym("_r_flw_njv_bgn"),
		Sym("_br_flw_njv_bgn"),
		Sym("_r_dqt_str_bgn"),
		Sym("_br_dqt_str_bgn"),
		Sym("_b_dqt_str_bgn"),
		Sym("_r_dqt_str_ctn"),
		Sym("_br_dqt_str_ctn"),
		Sym("_r_dqt_esc_nwl"),
		Sym("_br_dqt_esc_nwl"),
		Sym("_r_dqt_esc_seq"),
		Sym("_br_dqt_esc_seq"),
		Sym("_r_dqt_str_end"),
		Sym("_br_dqt_str_end"),
		Sym("_r_sqt_str_bgn"),
		Sym("_br_sqt_str_bgn"),
		Sym("_b_sqt_str_bgn"),
		Sym("_r_sqt_str_ctn"),
		Sym("_br_sqt_str_ctn"),
		Sym("_r_sqt_esc_sqt"),
		Sym("_br_sqt_esc_sqt"),
		Sym("_r_sqt_str_end"),
		Sym("_br_sqt_str_end"),
		Sym("_r_sgl_pln_nul_blk"),
		Sym("_br_sgl_pln_nul_blk"),
		Sym("_b_sgl_pln_nul_blk"),
		Sym("_r_sgl_pln_nul_flw"),
		Sym("_br_sgl_pln_nul_flw"),
		Sym("_r_sgl_pln_bol_blk"),
		Sym("_br_sgl_pln_bol_blk"),
		Sym("_b_sgl_pln_bol_blk"),
		Sym("_r_sgl_pln_bol_flw"),
		Sym("_br_sgl_pln_bol_flw"),
		Sym("_r_sgl_pln_int_blk"),
		Sym("_br_sgl_pln_int_blk"),
		Sym("_b_sgl_pln_int_blk"),
		Sym("_r_sgl_pln_int_flw"),
		Sym("_br_sgl_pln_int_flw"),
		Sym("_r_sgl_pln_flt_blk"),
		Sym("_br_sgl_pln_flt_blk"),
		Sym("_b_sgl_pln_flt_blk"),
		Sym("_r_sgl_pln_flt_flw"),
		Sym("_br_sgl_pln_flt_flw"),
		Sym("_r_sgl_pln_tms_blk"),
		Sym("_br_sgl_pln_tms_blk"),
		Sym("_b_sgl_pln_tms_blk"),
		Sym("_r_sgl_pln_tms_flw"),
		Sym("_br_sgl_pln_tms_flw"),
		Sym("_r_sgl_pln_str_blk"),
		Sym("_br_sgl_pln_str_blk"),
		Sym("_b_sgl_pln_str_blk"),
		Sym("_r_sgl_pln_str_flw"),
		Sym("_br_sgl_pln_str_flw"),
		Sym("_r_mtl_pln_str_blk"),
		Sym("_br_mtl_pln_str_blk"),
		Sym("_r_mtl_pln_str_flw"),
		Sym("_br_mtl_pln_str_flw"),
		Sym("_r_tag"),
		Sym("_br_tag"),
		Sym("_b_tag"),
		Sym("_r_acr_bgn"),
		Sym("_br_acr_bgn"),
		Sym("_b_acr_bgn"),
		Sym("_r_acr_ctn"),
		Sym("_r_als_bgn"),
		Sym("_br_als_bgn"),
		Sym("_b_als_bgn"),
		Sym("_r_als_ctn"),
		Sym("_bl"),
		Sym("comment"),
		Sym("_err_rec"),
	)

	g.SetInline("_r_pln_blk", "_br_pln_blk", "_r_pln_flw", "_br_pln_flw", "_r_blk_seq_val", "_r_blk_map_val", "_r_flw_val_blk", "_br_flw_val_blk", "_r_sgl_flw_val_blk", "_br_sgl_flw_val_blk", "_b_sgl_flw_val_blk", "_r_flw_val_flw", "_br_flw_val_flw", "_r_sgl_flw_val_flw", "_r_flw_jsl_val", "_br_flw_jsl_val", "_r_sgl_flw_jsl_val", "_br_sgl_flw_jsl_val", "_b_sgl_flw_jsl_val", "_r_flw_njl_val_blk", "_br_flw_njl_val_blk", "_r_sgl_flw_njl_val_blk", "_br_sgl_flw_njl_val_blk", "_b_sgl_flw_njl_val_blk", "_r_flw_njl_val_flw", "_br_flw_njl_val_flw", "_r_sgl_flw_njl_val_flw")

	// YAML's external scanner has 113 context-sensitive tokens. Preserve the
	// precise external lexer states so dedented quoted scalars retain context.
	g.PreferPreciseExternalLexStates = true
	// Minimize states only after conflict resolution preserves their behavior.
	g.CompactParseStates = true

	return g
}
