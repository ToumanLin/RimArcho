using System.Reflection;
namespace LogLoader {
  // Loads an assembly reflection-only (a mentioned candidate for missing assembly dependencies).
  public static class Loader {
    public static Assembly Load(string path) { return Assembly.ReflectionOnlyLoadFrom(path); }
  }
}
