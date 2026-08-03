// Merge-event census oracle, reference-runtime side (stage M0 of
// spec.merge-time-election.v1).
//
// The stage-D0 derivation-set differential reconstructs the reference
// runtime's live version set from ts_parser_set_logger output. That method
// cannot reach a merge event: parser.c calls ts_stack_merge at four sites
// (parser.c:1033, :1114, :1513, :1796/:1805) and NONE of them logs. The only
// merge-adjacent log line is "condense", which parser.c:1855 emits when
// made_changes is set by any of removal, merge, or swap, so it cannot be
// counted as a merge. The census therefore uses the instrumented build, which
// the repository already carries: cgo_harness/work_count/tree_sitter_v0_25_1.patch
// adds merge_attempts_proxy and merge_successes_proxy directly around
// ts_stack_merge (stack.c) and the six GTSLinkUnionOutcome counters around
// stack_node_add_link.
//
// This driver is compiled against the SAME runtime source the pinned C oracle
// uses (github.com/tree-sitter/go-tree-sitter@v0.25.0/src, resolved from the
// module cache and patched into a private snapshot) and it dlopens the SAME
// grammar shared object the parity C-reference loader built. Both sides of
// the census therefore run one runtime and one grammar table.
//
// Protocol: read a manifest of source paths, parse each with a fresh parser,
// print one JSON object per line. A per-source failure prints a status field
// and never aborts the batch.

#include <dlfcn.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "api.h"
#include "work_count.h"

typedef const TSLanguage *(*gts_lang_fn)(void);

static char *read_file(const char *path, size_t *out_len) {
  FILE *file = fopen(path, "rb");
  if (!file) return NULL;
  if (fseek(file, 0, SEEK_END) != 0) {
    fclose(file);
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

static void print_escaped(const char *value) {
  for (const char *p = value; *p; p++) {
    switch (*p) {
      case '"': fputs("\\\"", stdout); break;
      case '\\': fputs("\\\\", stdout); break;
      case '\n': fputs("\\n", stdout); break;
      case '\r': fputs("\\r", stdout); break;
      case '\t': fputs("\\t", stdout); break;
      default:
        if ((unsigned char)*p < 0x20) {
          printf("\\u%04x", (unsigned char)*p);
        } else {
          fputc(*p, stdout);
        }
    }
  }
}

static void print_row(const char *path, const char *status, GTSWorkCount c,
                      uint32_t source_len, uint32_t root_end_byte,
                      bool root_has_error) {
  printf("{\"schema\":\"gts-merge-census-c/v1\",\"path\":\"");
  print_escaped(path);
  printf("\",\"status\":\"%s\"", status);
  printf(",\"source_bytes\":%u,\"root_end_byte\":%u,\"root_has_error\":%s",
         source_len, root_end_byte, root_has_error ? "true" : "false");
#define ROW(field) printf(",\"" #field "\":%llu", (unsigned long long)c.field)
  ROW(merge_attempts_proxy);
  ROW(merge_successes_proxy);
  ROW(stack_version_creations_proxy);
  ROW(shifts);
  ROW(reductions);
  ROW(accept_actions);
  ROW(explicit_recover_actions);
  ROW(reduction_pop_requests);
  ROW(emitted_pop_paths);
  ROW(predecessor_link_union_attempts);
  ROW(predecessor_link_union_duplicate_noop);
  ROW(predecessor_link_union_precedence_replaced);
  ROW(predecessor_link_union_recursive_changed);
  ROW(predecessor_link_union_alternate_appended);
  ROW(predecessor_link_union_rejected);
  ROW(alternate_predecessor_links_appended);
  ROW(graph_link_additions_proxy);
#undef ROW
  printf(",\"overflow\":%s}\n", c.overflow ? "true" : "false");
}

static unsigned long long parse_u64(const char *raw) {
  char *end = NULL;
  unsigned long long value = strtoull(raw, &end, 10);
  if (!raw[0] || (end && *end)) return 0;
  return value;
}

int main(int argc, char **argv) {
  if (argc == 2 && strcmp(argv[1], "--exact-model") == 0) {
    // The same three self-checks the work-count oracle runs, so a census run
    // proves the counters it reads still behave as the patch's model states.
    bool passed = gts_work_count_validate_action_model() &&
                  gts_work_count_validate_link_union_model() &&
                  gts_work_count_validate_raw_census_model();
    printf("{\"schema\":\"gts-merge-census-c-exact-model/v1\",\"passed\":%s}\n",
           passed ? "true" : "false");
    return passed ? 0 : 12;
  }
  if (argc != 5) {
    fputs("status=c_protocol_error\n", stderr);
    fprintf(stderr, "usage: %s <grammar-so> <symbol> <manifest> <timeout-us>\n",
            argv[0]);
    return 2;
  }

  unsigned long long timeout_us = parse_u64(argv[4]);
  if (timeout_us == 0) {
    fputs("status=c_protocol_error\n", stderr);
    return 2;
  }

  void *handle = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
  if (!handle) {
    fprintf(stderr, "status=c_dlopen_error %s\n", dlerror());
    return 3;
  }
  // argv[2] is a comma-separated candidate list, mirroring the parity
  // loader's parityLanguageSymbols: a grammar repository does not always name
  // its entry point after the lock row.
  gts_lang_fn lang_fn = NULL;
  char *symbols = strdup(argv[2]);
  if (!symbols) {
    fputs("status=c_alloc_error\n", stderr);
    return 3;
  }
  for (char *candidate = strtok(symbols, ","); candidate;
       candidate = strtok(NULL, ",")) {
    dlerror();
    void *symbol = dlsym(handle, candidate);
    if (symbol) {
      lang_fn = (gts_lang_fn)symbol;
      break;
    }
  }
  free(symbols);
  if (!lang_fn) {
    fprintf(stderr, "status=c_dlsym_error %s\n", argv[2]);
    return 3;
  }
  const TSLanguage *language = lang_fn();
  if (!language) {
    fputs("status=c_language_error\n", stderr);
    return 3;
  }

  size_t manifest_len = 0;
  char *manifest = read_file(argv[3], &manifest_len);
  if (!manifest) {
    fputs("status=c_manifest_error\n", stderr);
    return 4;
  }

  char *cursor = manifest;
  while (cursor && *cursor) {
    char *newline = strchr(cursor, '\n');
    if (newline) *newline = '\0';
    char *path = cursor;
    cursor = newline ? newline + 1 : NULL;
    if (path[0] == '\0') continue;

    size_t source_len = 0;
    char *source = read_file(path, &source_len);
    if (!source || source_len > UINT32_MAX) {
      free(source);
      print_row(path, "source_error", (GTSWorkCount){0}, 0u, 0u, false);
      continue;
    }

    TSParser *parser = ts_parser_new();
    if (!parser || !ts_parser_set_language(parser, language)) {
      if (parser) ts_parser_delete(parser);
      free(source);
      print_row(path, "parser_error", (GTSWorkCount){0}, (uint32_t)source_len,
                0u, false);
      continue;
    }
    ts_parser_set_timeout_micros(parser, timeout_us);

    gts_work_count_reset();
    TSTree *tree =
        ts_parser_parse_string(parser, NULL, source, (uint32_t)source_len);
    GTSWorkCount counts = gts_work_count_snapshot();
    if (!tree) {
      ts_parser_delete(parser);
      free(source);
      print_row(path, "timeout", counts, (uint32_t)source_len, 0u, false);
      continue;
    }
    TSNode root = ts_tree_root_node(tree);
    print_row(path, "ok", counts, (uint32_t)source_len, ts_node_end_byte(root),
              ts_node_has_error(root));
    ts_tree_delete(tree);
    ts_parser_delete(parser);
    free(source);
  }

  free(manifest);
  fflush(stdout);
  return ferror(stdout) ? 6 : 0;
}
