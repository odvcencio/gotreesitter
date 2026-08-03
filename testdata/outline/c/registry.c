#include <stddef.h>

struct Registry {
    int count;
};

typedef struct Registry RegistryAlias;

enum Level {
    LEVEL_LOW,
    LEVEL_HIGH
};

int registry_count(struct Registry *registry) {
    return registry->count;
}

void registry_reset(struct Registry *registry) {
    registry->count = 0;
}
