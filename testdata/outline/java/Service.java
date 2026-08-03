package demo;

public interface Service {
    String name();
}

enum Level {
    LOW,
    HIGH;
}

record Point(int x, int y) {
}

public class Registry implements Service {
    private final String label;

    Registry(String label) {
        this.label = label;
    }

    public String name() {
        return label;
    }

    class Inner {
        int depth() {
            return 1;
        }
    }
}
