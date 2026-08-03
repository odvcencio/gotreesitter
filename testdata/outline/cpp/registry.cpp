namespace demo {

struct Point {
    int x;
    int y;
};

class Registry {
public:
    int count() const {
        return count_;
    }

    void reset() {
        count_ = 0;
    }

private:
    int count_ = 0;
};

int total(const Registry &registry) {
    return registry.count();
}

}  // namespace demo
