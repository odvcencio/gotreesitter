#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "api.h"
#include "work_count.h"

#ifndef TS_LANG_FN
#error "TS_LANG_FN macro is required"
#endif

extern const TSLanguage *TS_LANG_FN(void);

// Runtime 0.26 removed the parser timeout setter. Keep the diagnostic wall
// budget through the upstream progress callback, without patching the parser.
typedef struct {
  const char *source;
  uint32_t length;
  struct timespec start;
  uint64_t timeout_us;
} GTSParseInput;

static const char *gts_parse_read(void *payload, uint32_t offset,
                                TSPoint point, uint32_t *length) {
  (void)point;
  GTSParseInput *input = payload;
  *length = offset < input->length ? input->length - offset : 0;
  return input->source + (offset < input->length ? offset : input->length);
}

static bool gts_parse_progress(TSParseState *state) {
  GTSParseInput *input = state->payload;
  struct timespec now;
  if (clock_gettime(CLOCK_MONOTONIC, &now) != 0) return true;
  int64_t elapsed_ns = (int64_t)(now.tv_sec - input->start.tv_sec) * 1000000000 +
                       now.tv_nsec - input->start.tv_nsec;
  return elapsed_ns >= 0 && (uint64_t)elapsed_ns / 1000 >= input->timeout_us;
}

static TSTree *gts_parse_with_timeout(TSParser *parser, const char *source,
                                    uint32_t length, uint64_t timeout_us) {
  if (timeout_us == 0) return ts_parser_parse_string(parser, NULL, source, length);
  GTSParseInput payload = {.source = source, .length = length,
                          .timeout_us = timeout_us};
  if (clock_gettime(CLOCK_MONOTONIC, &payload.start) != 0) return NULL;
  TSInput input = {.payload = &payload, .read = gts_parse_read,
                   .encoding = TSInputEncodingUTF8};
  TSParseOptions options = {.payload = &payload,
                           .progress_callback = gts_parse_progress};
  return ts_parser_parse_with_options(parser, NULL, input, options);
}

static char *read_source(const char *path, size_t *out_len) {
  FILE *file = fopen(path, "rb");
  if (!file || fseek(file, 0, SEEK_END) != 0) {
    if (file) fclose(file);
    return NULL;
  }
  long end = ftell(file);
  if (end < 0 || fseek(file, 0, SEEK_SET) != 0) {
    fclose(file);
    return NULL;
  }
  char *buf = malloc((size_t)end + 1u);
  if (!buf) {
    fclose(file);
    return NULL;
  }
  size_t n = fread(buf, 1u, (size_t)end, file);
  if (n != (size_t)end || ferror(file)) {
    free(buf);
    fclose(file);
    return NULL;
  }
  fclose(file);
  buf[n] = '\0';
  *out_len = n;
  return buf;
}

static bool tree_complete(TSTree *tree, uint32_t source_len) {
  if (!tree) return false;
  TSNode root = ts_tree_root_node(tree);
  return !ts_node_is_null(root) && ts_node_end_byte(root) == source_len;
}

static bool write_u32(FILE *file, uint32_t value) {
  unsigned char bytes[4] = {
      (unsigned char)value, (unsigned char)(value >> 8),
      (unsigned char)(value >> 16), (unsigned char)(value >> 24)};
  return fwrite(bytes, 1u, sizeof(bytes), file) == sizeof(bytes);
}

static bool write_string(FILE *file, const char *value) {
  uint32_t len = value ? (uint32_t)strlen(value) : 0u;
  return write_u32(file, len) &&
         (len == 0 || fwrite(value, 1u, len, file) == len);
}

static bool write_node_header(FILE *file, TSNode node, const char *field_name) {
  uint32_t child_count = ts_node_child_count(node);
  gts_work_count_add(&gts_work_count_current.selected_nodes, 1);
  if (child_count == 0) {
    gts_work_count_add(&gts_work_count_current.selected_leaf_nodes, 1);
  } else {
    gts_work_count_add(&gts_work_count_current.selected_parent_nodes, 1);
  }

  if (!write_string(file, ts_node_type(node)) ||
      !write_string(file, field_name) ||
      !write_u32(file, ts_node_start_byte(node)) ||
      !write_u32(file, ts_node_end_byte(node))) {
    return false;
  }
  TSPoint start = ts_node_start_point(node);
  TSPoint end = ts_node_end_point(node);
  if (!write_u32(file, start.row) || !write_u32(file, start.column) ||
      !write_u32(file, end.row) || !write_u32(file, end.column)) {
    return false;
  }
  unsigned char flags = 0u;
  if (ts_node_is_named(node)) flags |= 1u << 0;
  if (ts_node_is_extra(node)) flags |= 1u << 1;
  if (ts_node_is_missing(node)) flags |= 1u << 2;
  if (ts_node_is_error(node)) flags |= 1u << 3;
  if (ts_node_has_error(node)) flags |= 1u << 4;
  return fwrite(&flags, 1u, 1u, file) == 1u &&
         write_u32(file, child_count);
}

static bool write_tree(FILE *file, TSNode root) {
  static const unsigned char magic[] = "gts-deep-tree-v1";
  if (fwrite(magic, 1u, sizeof(magic), file) != sizeof(magic) ||
      !write_node_header(file, root, NULL)) {
    return false;
  }
  TSTreeCursor cursor = ts_tree_cursor_new(root);
  for (;;) {
    if (ts_tree_cursor_goto_first_child(&cursor)) {
      if (!write_node_header(file, ts_tree_cursor_current_node(&cursor),
                             ts_tree_cursor_current_field_name(&cursor))) {
        ts_tree_cursor_delete(&cursor);
        return false;
      }
      continue;
    }
    while (!ts_tree_cursor_goto_next_sibling(&cursor)) {
      if (!ts_tree_cursor_goto_parent(&cursor)) {
        ts_tree_cursor_delete(&cursor);
        return !ferror(file);
      }
    }
    if (!write_node_header(file, ts_tree_cursor_current_node(&cursor),
                           ts_tree_cursor_current_field_name(&cursor))) {
      ts_tree_cursor_delete(&cursor);
      return false;
    }
  }
}

static unsigned long long parse_timeout(const char *raw) {
  char *end = NULL;
  unsigned long long value = strtoull(raw, &end, 10);
  if (!raw[0] || (end && *end)) return 0;
  return value;
}

static void print_counts(GTSWorkCount c, uint32_t source_len,
                         uint32_t root_end_byte, bool root_has_error) {
  printf("{\n");
  printf("  \"schema\": \"gts-work-count-c-child/v6\",\n");
  printf("  \"engine\": \"static-c-instrumented-glr\",\n");
  printf("  \"digest_format\": \"gts-deep-tree-v1\",\n");
  printf("  \"source_bytes\": %u,\n", source_len);
  printf("  \"root_end_byte\": %u,\n", root_end_byte);
  printf("  \"root_has_error\": %s,\n", root_has_error ? "true" : "false");
  printf("  \"board_direct\": {\n");
  printf("    \"schema\": \"gts-work-count-board-direct/v3\",\n");
  printf("    \"frontier_lexer_elections_available\": false,\n");
  printf("    \"frontier_lexer_elections\": 0,\n");
  printf("    \"per_version_lex_requests_available\": true,\n");
  printf("    \"per_version_lex_requests\": %llu,\n", (unsigned long long)c.per_version_lex_requests);
  printf("    \"raw_main_lexer_invocations\": %llu,\n", (unsigned long long)c.raw_main_lexer_invocations);
  printf("    \"resolved_action_cells_examined\": %llu,\n", (unsigned long long)c.resolved_action_cells_examined);
  printf("    \"raw_action_entries_beyond_first\": %llu,\n", (unsigned long long)c.raw_action_entries_beyond_first);
  printf("    \"conflict_action_arms_admitted\": %llu,\n", (unsigned long long)c.conflict_action_arms_admitted);
  printf("    \"causal_conflict_forks\": %llu,\n", (unsigned long long)c.causal_conflict_forks);
  printf("    \"predecessor_link_union_attempts\": %llu,\n", (unsigned long long)c.predecessor_link_union_attempts);
  printf("    \"predecessor_link_union_duplicate_noop\": %llu,\n", (unsigned long long)c.predecessor_link_union_duplicate_noop);
  printf("    \"predecessor_link_union_precedence_replaced\": %llu,\n", (unsigned long long)c.predecessor_link_union_precedence_replaced);
  printf("    \"predecessor_link_union_recursive_changed\": %llu,\n", (unsigned long long)c.predecessor_link_union_recursive_changed);
  printf("    \"predecessor_link_union_alternate_appended\": %llu,\n", (unsigned long long)c.predecessor_link_union_alternate_appended);
  printf("    \"predecessor_link_union_rejected\": %llu,\n", (unsigned long long)c.predecessor_link_union_rejected);
  printf("    \"alternate_predecessor_links_appended\": %llu,\n", (unsigned long long)c.alternate_predecessor_links_appended);
  printf("    \"raw_selected_internal_nodes\": %llu,\n", (unsigned long long)c.raw_selected_internal_nodes);
  printf("    \"raw_selected_internal_parent_occurrences\": %llu,\n", (unsigned long long)c.raw_selected_internal_parent_occurrences);
  printf("    \"raw_selected_internal_leaf_occurrences\": %llu,\n", (unsigned long long)c.raw_selected_internal_leaf_occurrences);
  printf("    \"overflow\": %s\n", c.overflow ? "true" : "false");
  printf("  },\n");
  printf("  \"counters\": {\n");
  printf("    \"contract\": \"gts-work-count/v2\",\n");
  printf("    \"overflow\": %s,\n", c.overflow ? "true" : "false");
#define PRINT_COUNT(field) \
  printf("    \"" #field "\": %llu,\n", (unsigned long long)c.field)
  PRINT_COUNT(shifts);
  PRINT_COUNT(reductions);
  PRINT_COUNT(accept_actions);
  PRINT_COUNT(explicit_recover_actions);
  PRINT_COUNT(reduction_pop_requests);
  PRINT_COUNT(emitted_pop_paths);
  PRINT_COUNT(emitted_pop_payloads);
  PRINT_COUNT(selected_nodes);
  PRINT_COUNT(selected_parent_nodes);
  PRINT_COUNT(selected_leaf_nodes);
  PRINT_COUNT(table_lookups_proxy);
  PRINT_COUNT(action_entries_examined_proxy);
  PRINT_COUNT(lexer_front_door_calls_proxy);
  PRINT_COUNT(stack_version_creations_proxy);
  PRINT_COUNT(merge_attempts_proxy);
  PRINT_COUNT(merge_successes_proxy);
  PRINT_COUNT(graph_link_additions_proxy);
  PRINT_COUNT(leaf_constructions_proxy);
  PRINT_COUNT(parent_constructions_proxy);
  PRINT_COUNT(pending_parent_constructions_proxy);
#undef PRINT_COUNT
  printf("    \"no_tree_parent_constructions_proxy\": %llu\n",
         (unsigned long long)c.no_tree_parent_constructions_proxy);
  printf("  }\n");
  printf("}\n");
}

int main(int argc, char **argv) {
  if (argc == 2 && strcmp(argv[1], "--exact-model") == 0) {
    bool passed = gts_work_count_validate_action_model() &&
                  gts_work_count_validate_link_union_model() &&
                  gts_work_count_validate_raw_census_model();
    printf("{\"schema\":\"gts-work-count-c-exact-model/v1\",\"passed\":%s}\n",
           passed ? "true" : "false");
    return passed ? 0 : 12;
  }
  if (argc != 4) {
    fputs("status=c_protocol_error\n", stderr);
    fprintf(stderr, "usage: %s <source> <digest-dump> <timeout-us>\n", argv[0]);
    return 2;
  }

  size_t source_len = 0;
  char *source = read_source(argv[1], &source_len);
  if (!source || source_len > UINT32_MAX) {
    fputs("status=c_transport_error\n", stderr);
    free(source);
    return 3;
  }
  unsigned long long timeout_us = parse_timeout(argv[3]);
  if (timeout_us == 0) {
    fputs("status=c_protocol_error\n", stderr);
    free(source);
    return 2;
  }

  TSParser *parser = ts_parser_new();
  if (!parser || !ts_parser_set_language(parser, TS_LANG_FN())) {
    fputs("status=c_parser_error\n", stderr);
    if (parser) ts_parser_delete(parser);
    free(source);
    return 4;
  }

  gts_work_count_reset();
  TSTree *tree = gts_parse_with_timeout(parser, source, (uint32_t)source_len, timeout_us);
  if (!tree) {
    fputs("status=c_timeout\n", stderr);
    ts_parser_delete(parser);
    free(source);
    return 10;
  }
  if (!tree_complete(tree, (uint32_t)source_len)) {
    fputs("status=c_incomplete\n", stderr);
    ts_tree_delete(tree);
    ts_parser_delete(parser);
    free(source);
    return 11;
  }

  TSNode root = ts_tree_root_node(tree);
  FILE *dump = fopen(argv[2], "wb");
  bool dump_ok = false;
  if (dump) {
    dump_ok = write_tree(dump, root);
    if (fclose(dump) != 0) dump_ok = false;
    dump = NULL;
  }
  if (!dump_ok) {
    fputs("status=c_digest_error\n", stderr);
    ts_tree_delete(tree);
    ts_parser_delete(parser);
    free(source);
    return 5;
  }
  GTSWorkCount counts = gts_work_count_snapshot();
  print_counts(counts, (uint32_t)source_len, ts_node_end_byte(root),
               ts_node_has_error(root));

  ts_tree_delete(tree);
  ts_parser_delete(parser);
  free(source);
  return ferror(stdout) ? 6 : 0;
}
