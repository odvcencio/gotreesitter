#include <stdbool.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

#include <tree_sitter/api.h>

extern const TSLanguage *tree_sitter_go(void);

enum {
  EXIT_COMMAND = 64,
  EXIT_SEMANTIC = 65,
  EXIT_IDENTITY = 66,
  EXIT_ITERATION_CAP = 67,
  EXIT_CLOCK = 68,
  EXIT_INTERNAL = 70,
};

#ifndef GTS_TIMING_ORACLE_MIN_ELAPSED_NS
#define GTS_TIMING_ORACLE_MIN_ELAPSED_NS UINT64_C(750000000)
#endif

static const uint64_t k_min_elapsed_ns = GTS_TIMING_ORACLE_MIN_ELAPSED_NS;
static const uint64_t k_max_iterations = UINT64_C(1000000000);
static const uint64_t k_fnv_offset = UINT64_C(14695981039346656037);
static const uint64_t k_fnv_prime = UINT64_C(1099511628211);
static const unsigned char k_deep_header[] = "gts-deep-tree-v1";

typedef enum {
  LANE_PUBLIC_VALIDATED,
  LANE_SELECTED_RELEASE,
  LANE_SELECTED_CONSUMER,
} lane_t;

typedef enum {
  OP_OK,
  OP_SEMANTIC,
  OP_INTERNAL,
} operation_result_t;

typedef struct {
  const char *name;
  uint32_t source_bytes;
  const char *source_sha256;
  uint64_t visited_nodes;
  uint64_t checksum;
} fixture_t;

static const fixture_t k_fixtures[] = {
    {"rewrite", UINT32_C(5116),
     "74c0705f8729670559492fb5460a01b2a1a2a109928e1aeb52736e485e8ff097",
     UINT64_C(1524), UINT64_C(0xc71076528146b3f8)},
    {"query_compile", UINT32_C(20168),
     "b788ee19b0075f0b9b567a9f93ea657e715bc8a6a40a99d3ca5c761404e71894",
     UINT64_C(7524), UINT64_C(0x9c30f7f940ebadf3)},
    {"language", UINT32_C(41387),
     "009aa9fd5352c712f3839670c7df8a9b00ae878ee20dc88131a438b2d5edfd9a",
     UINT64_C(7082), UINT64_C(0x41c2eb88ff43be57)},
    {"grammargen_lr", UINT32_C(235626),
     "a7e4a1a64b25a60aea36183b9d6d53dcd9240942cdb10e67a3cf9e6ce30f95b2",
     UINT64_C(71768), UINT64_C(0x4c3be1f65cf3aed4)},
};

typedef enum {
  SINK_STDOUT,
  SINK_FNV,
} sink_mode_t;

typedef struct {
  sink_mode_t mode;
  uint64_t fnv;
  bool failed;
} deep_sink_t;

typedef struct {
  uint32_t declared_children;
  uint32_t visited_children;
} cursor_frame_t;

typedef struct {
  uint64_t visited_nodes;
  uint64_t checksum;
} consumer_result_t;

static void fail_message(const char *message) {
  (void)fprintf(stderr, "go_timing_oracle: %s\n", message);
}

static const fixture_t *fixture_by_name(const char *name) {
  size_t count = sizeof(k_fixtures) / sizeof(k_fixtures[0]);
  for (size_t i = 0; i < count; i++) {
    if (strcmp(name, k_fixtures[i].name) == 0) {
      return &k_fixtures[i];
    }
  }
  return NULL;
}

static bool lane_by_name(const char *name, lane_t *lane) {
  if (strcmp(name, "public_validated") == 0) {
    *lane = LANE_PUBLIC_VALIDATED;
    return true;
  }
  if (strcmp(name, "selected_release") == 0) {
    *lane = LANE_SELECTED_RELEASE;
    return true;
  }
  if (strcmp(name, "selected_consumer") == 0) {
    *lane = LANE_SELECTED_CONSUMER;
    return true;
  }
  return false;
}

static const char *lane_name(lane_t lane) {
  switch (lane) {
    case LANE_PUBLIC_VALIDATED:
      return "public_validated";
    case LANE_SELECTED_RELEASE:
      return "selected_release";
    case LANE_SELECTED_CONSUMER:
      return "selected_consumer";
  }
  return "";
}

static bool parse_source_fd(const char *value, int *fd_out) {
  if (value == NULL || value[0] == '\0' ||
      (value[0] == '0' && value[1] != '\0')) {
    return false;
  }
  char *end = NULL;
  errno = 0;
  long parsed = strtol(value, &end, 10);
  if (errno != 0 || end == value || *end != '\0' || parsed < 3 ||
      parsed > INT_MAX) {
    return false;
  }
  *fd_out = (int)parsed;
  return true;
}

/* Consume only the collector-retained, O_NOFOLLOW-opened inherited fixture. */
static char *read_source_fd(int fd, size_t *out_len) {
  struct stat identity;
  int descriptor_flags = fcntl(fd, F_GETFD);
  int status_flags = fcntl(fd, F_GETFL);
  if (descriptor_flags < 0 || status_flags < 0 ||
      (descriptor_flags & FD_CLOEXEC) != 0 ||
      (status_flags & O_ACCMODE) != O_RDONLY || fstat(fd, &identity) != 0 ||
      !S_ISREG(identity.st_mode) || identity.st_size <= 0 ||
      (uintmax_t)identity.st_size > UINT32_MAX) {
    return NULL;
  }
  size_t length = (size_t)identity.st_size;
  char *source = (char *)malloc(length + 1u);
  if (source == NULL) {
    return NULL;
  }
  size_t offset = 0;
  while (offset < length) {
    ssize_t count = pread(fd, source + offset, length - offset, (off_t)offset);
    if (count <= 0) {
      free(source);
      return NULL;
    }
    offset += (size_t)count;
  }
  source[length] = '\0';
  *out_len = length;
  return source;
}

static bool monotonic_now(uint64_t *out) {
  struct timespec value;
  if (clock_gettime(CLOCK_MONOTONIC, &value) != 0 || value.tv_sec < 0 ||
      value.tv_nsec < 0 || value.tv_nsec >= 1000000000L) {
    return false;
  }
  uint64_t seconds = (uint64_t)value.tv_sec;
  if (seconds > (UINT64_MAX - (uint64_t)value.tv_nsec) /
                    UINT64_C(1000000000)) {
    return false;
  }
  *out = seconds * UINT64_C(1000000000) + (uint64_t)value.tv_nsec;
  return true;
}

static bool monotonic_delta(uint64_t start, uint64_t end, uint64_t *elapsed) {
  if (end < start) {
    return false;
  }
  *elapsed = end - start;
  return true;
}

static void sink_bytes(deep_sink_t *sink, const void *data, size_t length) {
  if (sink->failed || length == 0) {
    return;
  }
  const unsigned char *bytes = (const unsigned char *)data;
  if (sink->mode == SINK_STDOUT) {
    if (fwrite(bytes, 1u, length, stdout) != length) {
      sink->failed = true;
    }
    return;
  }
  for (size_t i = 0; i < length; i++) {
    sink->fnv ^= (uint64_t)bytes[i];
    sink->fnv *= k_fnv_prime;
  }
}

static void sink_u32(deep_sink_t *sink, uint32_t value) {
  const unsigned char bytes[4] = {
      (unsigned char)value,
      (unsigned char)(value >> 8),
      (unsigned char)(value >> 16),
      (unsigned char)(value >> 24),
  };
  sink_bytes(sink, bytes, sizeof(bytes));
}

static void sink_string(deep_sink_t *sink, const char *value) {
  size_t length = value == NULL ? 0u : strlen(value);
  if (length > UINT32_MAX) {
    sink->failed = true;
    return;
  }
  sink_u32(sink, (uint32_t)length);
  sink_bytes(sink, value, length);
}

static void sink_node(deep_sink_t *sink, TSNode node, const char *field_name,
                      uint32_t child_count) {
  sink_string(sink, ts_node_type(node));
  sink_string(sink, field_name);
  sink_u32(sink, ts_node_start_byte(node));
  sink_u32(sink, ts_node_end_byte(node));
  TSPoint start = ts_node_start_point(node);
  TSPoint end = ts_node_end_point(node);
  sink_u32(sink, start.row);
  sink_u32(sink, start.column);
  sink_u32(sink, end.row);
  sink_u32(sink, end.column);
  unsigned char flags = 0u;
  if (ts_node_is_named(node)) {
    flags |= 1u << 0;
  }
  if (ts_node_is_extra(node)) {
    flags |= 1u << 1;
  }
  sink_bytes(sink, &flags, 1u);
  sink_u32(sink, child_count);
}

/*
 * Feed the normative deep-tree stream while proving that cursor navigation
 * visits exactly each node's declared number of children and exhausts at the
 * root. Frames are bounded by the admitted fixture node count.
 */
static bool consume_tree(TSNode root, deep_sink_t *sink, uint64_t node_limit,
                         cursor_frame_t *frames, size_t frame_capacity,
                         uint64_t *visited_out) {
  if (ts_node_is_null(root) || node_limit == 0 || frames == NULL ||
      frame_capacity == 0) {
    return false;
  }

  TSTreeCursor cursor = ts_tree_cursor_new(root);
  uint64_t visited = 0;
  uint64_t depth = 0;
  bool need_visit = true;
  bool ok = true;

  sink_bytes(sink, k_deep_header, sizeof(k_deep_header));
  while (ok) {
    if (need_visit) {
      if (visited == node_limit || depth > node_limit) {
        ok = false;
        break;
      }
      TSNode node = ts_tree_cursor_current_node(&cursor);
      if (ts_node_is_null(node)) {
        ok = false;
        break;
      }
      uint32_t child_count = ts_node_child_count(node);
      const char *field_name =
          depth == 0 ? NULL : ts_tree_cursor_current_field_name(&cursor);
      frames[depth].declared_children = child_count;
      frames[depth].visited_children = 0;
      sink_node(sink, node, field_name, child_count);
      visited++;
      if (sink->failed) {
        ok = false;
        break;
      }
      if (child_count != 0) {
        if (!ts_tree_cursor_goto_first_child(&cursor)) {
          ok = false;
          break;
        }
        frames[depth].visited_children = 1;
        depth++;
        if (depth >= frame_capacity) {
          ok = false;
          break;
        }
        need_visit = true;
        continue;
      }
      if (ts_tree_cursor_goto_first_child(&cursor)) {
        ok = false;
        break;
      }
      need_visit = false;
    }

    if (frames[depth].visited_children != frames[depth].declared_children) {
      ok = false;
      break;
    }
    if (depth == 0) {
      if (ts_tree_cursor_goto_next_sibling(&cursor) ||
          ts_tree_cursor_goto_parent(&cursor)) {
        ok = false;
      }
      break;
    }
    if (ts_tree_cursor_goto_next_sibling(&cursor)) {
      uint64_t parent = depth - 1u;
      if (frames[parent].visited_children == UINT32_MAX) {
        ok = false;
        break;
      }
      frames[parent].visited_children++;
      if (frames[parent].visited_children > frames[parent].declared_children) {
        ok = false;
        break;
      }
      need_visit = true;
      continue;
    }
    if (!ts_tree_cursor_goto_parent(&cursor)) {
      ok = false;
      break;
    }
    depth--;
    need_visit = false;
  }

  ts_tree_cursor_delete(&cursor);
  if (!ok || visited == 0) {
    return false;
  }
  *visited_out = visited;
  return true;
}

static bool selected_root_valid(TSTree *tree, uint32_t source_len,
                                TSNode *root_out) {
  if (tree == NULL) {
    return false;
  }
  TSNode root = ts_tree_root_node(tree);
  if (ts_node_is_null(root) || ts_node_start_byte(root) != 0 ||
      ts_node_end_byte(root) != source_len) {
    return false;
  }
  *root_out = root;
  return true;
}

static bool public_root_valid(TSTree *tree, uint32_t source_len,
                              TSNode *root_out) {
  if (!selected_root_valid(tree, source_len, root_out)) {
    return false;
  }
  return !ts_node_has_error(*root_out);
}

static operation_result_t perform_lane(TSParser *parser, lane_t lane,
                                       const char *source, uint32_t source_len,
                                       uint64_t node_limit,
                                       cursor_frame_t *frames,
                                       size_t frame_capacity,
                                       consumer_result_t *result) {
  result->visited_nodes = 0;
  result->checksum = 0;
  TSTree *tree = ts_parser_parse_string(parser, NULL, source, source_len);
  TSNode root;
  bool valid = lane == LANE_PUBLIC_VALIDATED
                   ? public_root_valid(tree, source_len, &root)
                   : selected_root_valid(tree, source_len, &root);
  if (!valid) {
    if (tree != NULL) {
      ts_tree_delete(tree);
    }
    return OP_SEMANTIC;
  }

  bool ok = true;
  if (lane == LANE_SELECTED_CONSUMER) {
    deep_sink_t sink = {.mode = SINK_FNV, .fnv = k_fnv_offset, .failed = false};
    ok = consume_tree(root, &sink, node_limit, frames, frame_capacity,
                      &result->visited_nodes);
    result->checksum = sink.fnv;
  }
  ts_tree_delete(tree);
  return ok ? OP_OK : OP_SEMANTIC;
}

static int dump_deep(int source_fd) {
  size_t source_len = 0;
  char *source = read_source_fd(source_fd, &source_len);
  if (source == NULL) {
    fail_message("source must be a readable non-empty file no larger than UINT32_MAX");
    return EXIT_COMMAND;
  }
  TSParser *parser = ts_parser_new();
  if (parser == NULL || !ts_parser_set_language(parser, tree_sitter_go())) {
    fail_message("parser initialization failed");
    if (parser != NULL) {
      ts_parser_delete(parser);
    }
    free(source);
    return EXIT_INTERNAL;
  }
  TSTree *tree =
      ts_parser_parse_string(parser, NULL, source, (uint32_t)source_len);
  TSNode root;
  if (!public_root_valid(tree, (uint32_t)source_len, &root)) {
    fail_message("deep tree failed public completeness validation");
    if (tree != NULL) {
      ts_tree_delete(tree);
    }
    ts_parser_delete(parser);
    free(source);
    return EXIT_SEMANTIC;
  }
  deep_sink_t sink = {.mode = SINK_STDOUT, .fnv = 0, .failed = false};
  uint64_t visited = 0;
  size_t frame_capacity = (size_t)source_len + 1u;
  cursor_frame_t *frames = calloc(frame_capacity, sizeof(*frames));
  bool ok = frames != NULL &&
            consume_tree(root, &sink, UINT32_MAX, frames, frame_capacity,
                         &visited);
  free(frames);
  ts_tree_delete(tree);
  ts_parser_delete(parser);
  free(source);
  if (!ok || sink.failed || ferror(stdout) || fflush(stdout) != 0) {
    fail_message("deep stream write or traversal failed");
    return EXIT_INTERNAL;
  }
  return 0;
}

static int benchmark(lane_t lane, const fixture_t *fixture,
                     int source_fd) {
  size_t source_len = 0;
  char *source = read_source_fd(source_fd, &source_len);
  if (source == NULL) {
    fail_message("source must be a readable non-empty file no larger than UINT32_MAX");
    return EXIT_COMMAND;
  }
  if (source_len != fixture->source_bytes) {
    fail_message("source byte count does not match the sealed fixture");
    free(source);
    return EXIT_IDENTITY;
  }

  TSParser *parser = ts_parser_new();
  if (parser == NULL || !ts_parser_set_language(parser, tree_sitter_go())) {
    fail_message("parser initialization failed");
    if (parser != NULL) {
      ts_parser_delete(parser);
    }
    free(source);
    return EXIT_INTERNAL;
  }

  size_t frame_capacity = lane == LANE_SELECTED_CONSUMER
                              ? (size_t)fixture->visited_nodes + 1u
                              : 1u;
  cursor_frame_t *frames = calloc(frame_capacity, sizeof(*frames));
  if (frames == NULL) {
    fail_message("consumer proof scratch allocation failed");
    ts_parser_delete(parser);
    free(source);
    return EXIT_INTERNAL;
  }

  consumer_result_t warm;
  operation_result_t warm_status =
      perform_lane(parser, lane, source, (uint32_t)source_len,
                   fixture->visited_nodes, frames, frame_capacity, &warm);
  if (warm_status != OP_OK) {
    fail_message("warm lane operation failed");
    free(frames);
    ts_parser_delete(parser);
    free(source);
    return EXIT_SEMANTIC;
  }
  if (lane == LANE_SELECTED_CONSUMER &&
      (warm.visited_nodes != fixture->visited_nodes ||
       warm.checksum != fixture->checksum)) {
    fail_message("warm consumer count or checksum mismatch");
    free(frames);
    ts_parser_delete(parser);
    free(source);
    return EXIT_SEMANTIC;
  }

  uint64_t start = 0;
  uint64_t end = 0;
  uint64_t elapsed = 0;
  uint64_t iterations = 0;
  unsigned int sticky_mismatch = 0u;
  operation_result_t operation_status = OP_OK;
  bool iteration_cap_failed = false;
  bool clock_failed = !monotonic_now(&start);
  while (!clock_failed && operation_status == OP_OK &&
         !iteration_cap_failed) {
    consumer_result_t result;
    operation_status = perform_lane(parser, lane, source, (uint32_t)source_len,
                                    fixture->visited_nodes, frames,
                                    frame_capacity, &result);
    if (operation_status == OP_OK && lane == LANE_SELECTED_CONSUMER) {
      sticky_mismatch |= result.visited_nodes != fixture->visited_nodes;
      sticky_mismatch |= result.checksum != fixture->checksum;
    }
    if (operation_status == OP_OK) {
      if (iterations == UINT64_MAX) {
        operation_status = OP_INTERNAL;
      } else {
        iterations++;
      }
    }
    if (!monotonic_now(&end) || !monotonic_delta(start, end, &elapsed)) {
      clock_failed = true;
      break;
    }
    if (operation_status != OP_OK || elapsed >= k_min_elapsed_ns) {
      break;
    }
    if (iterations == k_max_iterations) {
      iteration_cap_failed = true;
      break;
    }
  }

  free(frames);
  ts_parser_delete(parser);
  free(source);
  if (operation_status == OP_SEMANTIC || sticky_mismatch != 0u) {
    fail_message("timed lane operation or sticky semantic check failed");
    return EXIT_SEMANTIC;
  }
  if (operation_status == OP_INTERNAL) {
    fail_message("timed lane internal failure");
    return EXIT_INTERNAL;
  }
  if (iteration_cap_failed) {
    fail_message("iteration cap reached before minimum duration");
    return EXIT_ITERATION_CAP;
  }
  if (clock_failed || elapsed < k_min_elapsed_ns || iterations == 0) {
    fail_message("monotonic clock contract failed");
    return EXIT_CLOCK;
  }

  uint64_t visited =
      lane == LANE_SELECTED_CONSUMER ? fixture->visited_nodes : 0;
  uint64_t checksum =
      lane == LANE_SELECTED_CONSUMER ? fixture->checksum : 0;
  char sample[1024];
  int length = snprintf(
      sample, sizeof(sample),
      "schema=gts-go-c-full-parse-sample/v2\n"
      "status=ok\n"
      "implementation=c\n"
      "lane=%s\n"
      "fixture=%s\n"
      "source_sha256=%s\n"
      "source_bytes=%u\n"
      "clock=clock-monotonic\n"
      "warmups=1\n"
      "iterations=%llu\n"
      "elapsed_ns=%llu\n"
      "visited_nodes=%llu\n"
      "traversal_checksum=%016llx\n",
      lane_name(lane), fixture->name, fixture->source_sha256,
      fixture->source_bytes, (unsigned long long)iterations,
      (unsigned long long)elapsed, (unsigned long long)visited,
      (unsigned long long)checksum);
  if (length <= 0 || (size_t)length >= sizeof(sample) || length > PIPE_BUF ||
      write(STDOUT_FILENO, sample, (size_t)length) != (ssize_t)length) {
    fail_message("sample write failed");
    return EXIT_INTERNAL;
  }
  return 0;
}

static void usage(const char *program) {
  (void)fprintf(
      stderr,
      "usage: %s --dump-deep --source-fd FD\n"
      "       %s --bench --lane public_validated|selected_release|selected_consumer "
      "--fixture rewrite|query_compile|language|grammargen_lr --source-fd FD\n",
      program, program);
}

int main(int argc, char **argv) {
  if (argc == 4 && strcmp(argv[1], "--dump-deep") == 0 &&
      strcmp(argv[2], "--source-fd") == 0) {
    int source_fd;
    if (!parse_source_fd(argv[3], &source_fd)) {
      usage(argv[0]);
      return EXIT_COMMAND;
    }
    return dump_deep(source_fd);
  }
  if (argc == 8 && strcmp(argv[1], "--bench") == 0 &&
      strcmp(argv[2], "--lane") == 0 &&
      strcmp(argv[4], "--fixture") == 0 &&
      strcmp(argv[6], "--source-fd") == 0 && argv[3][0] != '\0' &&
      argv[5][0] != '\0' && argv[7][0] != '\0') {
    lane_t lane;
    int source_fd;
    const fixture_t *fixture = fixture_by_name(argv[5]);
    if (!lane_by_name(argv[3], &lane) || fixture == NULL ||
        !parse_source_fd(argv[7], &source_fd)) {
      usage(argv[0]);
      return EXIT_COMMAND;
    }
    return benchmark(lane, fixture, source_fd);
  }
  usage(argv[0]);
  return EXIT_COMMAND;
}
