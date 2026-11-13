template<typename T>
void f(T&&) requires Eq<T>; // can appear as the last element of a function declarator

template<typename T> requires Addable<T> // or right after a template parameter list
T add(T a, T b) { return a + b; }

template<typename T>
concept Addable = requires (T x) { x + x; }; // requires-expression

template<typename T>
    requires requires (T x) { x + x; } // ad-hoc constraint, note keyword used twice
T add(T a, T b) { return a + b; }

template<typename T>
    requires (!std::is_same_v<T, bool>) // parenthesized expressions are allowed
void f(T);

template<typename T requires Addable<T> && Subtractable<T> // conjunctions
T f(T);

template<typename T> requires Addable<T> and Subtractable<T> // conjunctions
T f(T);

template<typename T> requires Addable<T> || Subtractable<T> // disjunctions
T f(T);

template<typename T> requires Addable<T> or Subtractable<T> // conjunctions
T f(T);

template<typename T> requires false || true // boolean literals
T f(T);

template<typename... T> requires (... && Addable<T>) // fold expressions
T f(T);
