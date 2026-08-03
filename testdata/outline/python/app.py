CONSTANT = 3


def top_level(value):
    def helper(inner):
        return inner + 1

    return helper(value)


class Service:
    def __init__(self, name):
        self.name = name

    def render(self):
        class Nested:
            def deep(self):
                return 1

        return Nested().deep()


class Empty:
    pass
