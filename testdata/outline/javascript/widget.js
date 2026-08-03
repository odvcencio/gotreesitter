function build(options) {
  function normalize(value) {
    return String(value);
  }
  return normalize(options.title);
}

class Widget {
  constructor(title) {
    this.title = title;
  }

  render() {
    function inner() {
      return 1;
    }
    return inner();
  }
}

class Empty {}
