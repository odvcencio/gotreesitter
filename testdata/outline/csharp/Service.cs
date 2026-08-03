namespace Demo
{
    public interface IService
    {
        string Name();
    }

    public class Registry : IService
    {
        private readonly string label;

        public Registry(string label)
        {
            this.label = label;
        }

        public string Name()
        {
            return label;
        }

        public Uri PathToUri(string path)
        {
            return new Uri(path);
        }
    }

    public enum Level
    {
        Low,
        High
    }
}
