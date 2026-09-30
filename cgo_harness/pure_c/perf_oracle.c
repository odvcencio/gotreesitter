#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "api.h"

#ifndef TS_LANG_FN
#error "TS_LANG_FN macro is required"
#endif

extern const TSLanguage *TS_LANG_FN(void);

static uint64_t now_ns(void) {
  struct timespec ts;
  clock_gettime(CLOCK_MONOTONIC, &ts);
  return (uint64_t)ts.tv_sec * 1000000000ull + (uint64_t)ts.tv_nsec;
}

// Keep the per-attempt timeout through the upstream progress callback.
// The runtime no longer exposes ts_parser_set_timeout_micros.
typedef struct {
  const char *source;
  uint32_t length;
  uint64_t start_ns;
  uint64_t timeout_us;
} ParseInput;

static const char *parse_read(void *payload, uint32_t offset,
                              TSPoint point, uint32_t *length) {
  (void)point;
  ParseInput *input = payload;
  *length = offset < input->length ? input->length - offset : 0;
  return input->source + (offset < input->length ? offset : input->length);
}

static bool parse_progress(TSParseState *state) {
  ParseInput *input = state->payload;
  return (now_ns() - input->start_ns) / 1000 >= input->timeout_us;
}

static TSTree *parse_with_timeout(TSParser *parser, const char *source,
                                 uint32_t length, uint64_t timeout_us) {
  if (timeout_us == 0) return ts_parser_parse_string(parser, NULL, source, length);
  ParseInput payload = {.source = source, .length = length,
                        .start_ns = now_ns(), .timeout_us = timeout_us};
  TSInput input = {.payload = &payload, .read = parse_read,
                   .encoding = TSInputEncodingUTF8};
  TSParseOptions options = {.payload = &payload,
                           .progress_callback = parse_progress};
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
  char *buf = (char *)malloc((size_t)end + 1u);
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

static void dump_u32(uint32_t value) {
  unsigned char bytes[4] = {
      (unsigned char)value, (unsigned char)(value >> 8),
      (unsigned char)(value >> 16), (unsigned char)(value >> 24)};
  fwrite(bytes, 1u, sizeof(bytes), stdout);
}

static void dump_string(const char *value) {
  uint32_t len = value ? (uint32_t)strlen(value) : 0u;
  dump_u32(len);
  if (len > 0) fwrite(value, 1u, len, stdout);
}

static void dump_node_header(TSNode node, const char *field_name) {
  dump_string(ts_node_type(node));
  dump_string(field_name);
  dump_u32(ts_node_start_byte(node));
  dump_u32(ts_node_end_byte(node));
  TSPoint start = ts_node_start_point(node);
  TSPoint end = ts_node_end_point(node);
  dump_u32(start.row);
  dump_u32(start.column);
  dump_u32(end.row);
  dump_u32(end.column);
  unsigned char flags = 0u;
  if (ts_node_is_named(node)) flags |= 1u << 0;
  if (ts_node_is_extra(node)) flags |= 1u << 1;
  if (ts_node_is_missing(node)) flags |= 1u << 2;
  if (ts_node_is_error(node)) flags |= 1u << 3;
  if (ts_node_has_error(node)) flags |= 1u << 4;
  fwrite(&flags, 1u, 1u, stdout);
  uint32_t count = ts_node_child_count(node);
  dump_u32(count);
}

static void dump_tree_nodes(TSNode root) {
  TSTreeCursor cursor = ts_tree_cursor_new(root);
  dump_node_header(root, NULL);
  for (;;) {
    if (ts_tree_cursor_goto_first_child(&cursor)) {
      dump_node_header(ts_tree_cursor_current_node(&cursor),
                       ts_tree_cursor_current_field_name(&cursor));
      continue;
    }
    while (!ts_tree_cursor_goto_next_sibling(&cursor)) {
      if (!ts_tree_cursor_goto_parent(&cursor)) {
        ts_tree_cursor_delete(&cursor);
        return;
      }
    }
    dump_node_header(ts_tree_cursor_current_node(&cursor),
                     ts_tree_cursor_current_field_name(&cursor));
  }
}

static int dump_tree(const char *source, uint32_t source_len,
                     uint64_t timeout_us) {
  TSParser *parser = ts_parser_new();
  if (!parser || !ts_parser_set_language(parser, TS_LANG_FN())) {
    fputs("status=c_parser_error\n", stderr);
    if (parser) ts_parser_delete(parser);
    return 2;
  }
  TSTree *tree = parse_with_timeout(parser, source, source_len, timeout_us);
  if (!tree) {
    fputs("status=c_timeout\n", stderr);
    ts_parser_delete(parser);
    return 10;
  }
  if (!tree_complete(tree, source_len)) {
    fputs("status=c_incomplete\n", stderr);
    if (tree) ts_tree_delete(tree);
    ts_parser_delete(parser);
    return 11;
  }
  static const unsigned char magic[] = "gts-deep-tree-v1";
  fwrite(magic, 1u, sizeof(magic), stdout);
  dump_tree_nodes(ts_tree_root_node(tree));
  ts_tree_delete(tree);
  ts_parser_delete(parser);
  if (ferror(stdout)) {
    fputs("status=c_digest_error\n", stderr);
    return 4;
  }
  return 0;
}

static int parse_int(const char *raw, int fallback) {
  char *end = NULL;
  long value = strtol(raw, &end, 10);
  if (!raw[0] || (end && *end) || value < 0 || value > 1000000000L) {
    return fallback;
  }
  return (int)value;
}

static int measure_full(TSParser *parser, const char *source, uint32_t len,
                        int warmup, int reps, uint64_t timeout_us) {
  for (int i = 0; i < warmup; i++) {
    TSTree *tree = parse_with_timeout(parser, source, len, timeout_us);
    if (!tree) {
      puts("status=c_timeout");
      return 0;
    }
    if (!tree_complete(tree, len)) {
      ts_tree_delete(tree);
      puts("status=c_incomplete");
      return 0;
    }
    ts_tree_delete(tree);
  }
  for (int i = 0; i < reps; i++) {
    uint64_t start = now_ns();
    TSTree *tree = parse_with_timeout(parser, source, len, timeout_us);
    uint64_t elapsed = now_ns() - start;
    if (!tree) {
      puts("status=c_timeout");
      return 0;
    }
    if (!tree_complete(tree, len)) {
      ts_tree_delete(tree);
      puts("status=c_incomplete");
      return 0;
    }
    ts_tree_delete(tree);
    printf("sample_ns=%llu\n", (unsigned long long)elapsed);
  }
  puts("status=ok");
  return 0;
}

int main(int argc, char **argv) {
  if (argc < 3) {
    fputs("status=c_protocol_error\n", stderr);
    fprintf(stderr, "usage: %s dump <source> <timeout-us> | measure full <source> <warmup> <reps> <timeout-us>\n", argv[0]);
    return 2;
  }
  size_t source_len = 0;
  const char *source_path = strcmp(argv[1], "dump") == 0 ? argv[2] : (argc >= 4 ? argv[3] : "");
  char *source = read_source(source_path, &source_len);
  if (!source || source_len > UINT32_MAX) {
    fputs("status=c_transport_error\n", stderr);
    fprintf(stderr, "source load failed: %s\n", source_path);
    free(source);
    return 2;
  }
  if (strcmp(argv[1], "dump") == 0) {
    if (argc != 4) {
      fputs("status=c_protocol_error\n", stderr);
      fprintf(stderr, "invalid dump command\n");
      free(source);
      return 2;
    }
    int timeout_us = parse_int(argv[3], -1);
    if (timeout_us < 0) {
      fputs("status=c_protocol_error\n", stderr);
      fprintf(stderr, "invalid dump timeout\n");
      free(source);
      return 2;
    }
    int status = dump_tree(source, (uint32_t)source_len,
                           (uint64_t)timeout_us);
    free(source);
    return status;
  }
  if (strcmp(argv[1], "measure") != 0 || argc != 7) {
    fputs("status=c_protocol_error\n", stderr);
    fprintf(stderr, "invalid measurement command\n");
    free(source);
    return 2;
  }
  const char *axis = argv[2];
  int warmup = parse_int(argv[4], -1);
  int reps = parse_int(argv[5], -1);
  int timeout_us = parse_int(argv[6], -1);
  if (warmup < 0 || reps < 1 || timeout_us < 0) {
    fputs("status=c_protocol_error\n", stderr);
    fprintf(stderr, "invalid warmup/reps/timeout\n");
    free(source);
    return 2;
  }
  TSParser *parser = ts_parser_new();
  if (!parser || !ts_parser_set_language(parser, TS_LANG_FN())) {
    fputs("status=c_parser_error\n", stderr);
    fprintf(stderr, "parser init failed\n");
    if (parser) ts_parser_delete(parser);
    free(source);
    return 2;
  }
  printf("schema=gts-static-c-perf/v2\naxis=%s\n", axis);
  int status = 2;
  if (strcmp(axis, "full") == 0) {
    status = measure_full(parser, source, (uint32_t)source_len, warmup, reps,
                          (uint64_t)timeout_us);
  } else {
    fputs("status=c_protocol_error\n", stderr);
    fprintf(stderr, "unsupported axis: %s\n", axis);
  }
  ts_parser_delete(parser);
  free(source);
  return status;
}
