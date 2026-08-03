pub struct Registry {
    count: usize,
}

pub enum Level {
    Low,
    High,
}

pub trait Named {
    fn name(&self) -> String;
}

impl Named for Registry {
    fn name(&self) -> String {
        String::from("registry")
    }
}

pub fn describe(registry: &Registry) -> String {
    fn suffix() -> &'static str {
        "!"
    }
    format!("{}{}", registry.name(), suffix())
}
