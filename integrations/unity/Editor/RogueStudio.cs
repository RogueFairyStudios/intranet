using System;
using System.Diagnostics;
using System.IO;
using System.Text;
using System.Threading.Tasks;
using UnityEditor;
using UnityEngine;

public static class RogueStudio
{
    private static string Setting(string name) { return Environment.GetEnvironmentVariable(name) ?? ""; }

    private static string Quote(string value)
    {
        if (value.IndexOfAny(new[] { '\r', '\n', '\0' }) >= 0) throw new ArgumentException("Invalid command argument");
        var result = new StringBuilder("\"");
        int slashes = 0;
        foreach (char character in value)
        {
            if (character == '\\') { slashes++; continue; }
            if (character == '"') result.Append('\\', slashes * 2 + 1);
            else result.Append('\\', slashes);
            result.Append(character);
            slashes = 0;
        }
        result.Append('\\', slashes * 2);
        return result.Append('"').ToString();
    }

    private static async void Run(params string[] arguments)
    {
        string script = Setting("STUDIO_CLIENT_PATH");
        if (!File.Exists(script)) { EditorUtility.DisplayDialog("Studio", "Set STUDIO_CLIENT_PATH before starting Unity.", "OK"); return; }
        string python = Setting("STUDIO_PYTHON");
        if (string.IsNullOrEmpty(python)) python = "python";
        var start = new ProcessStartInfo(python) { UseShellExecute = false, CreateNoWindow = true, Arguments = Quote(script) + " " + string.Join(" ", Array.ConvertAll(arguments, Quote)) };
        try
        {
            int exitCode = await Task.Run(() =>
            {
                using (var process = Process.Start(start)) { process.WaitForExit(); return process.ExitCode; }
            });
            EditorUtility.DisplayDialog("Studio", exitCode == 0 ? "Studio operation completed." : "Operation pending or failed. Check the Studio outbox.", "OK");
            AssetDatabase.Refresh();
        }
        catch (Exception) { EditorUtility.DisplayDialog("Studio", "Could not start the Studio client. Check Python and client paths.", "OK"); }
    }

    [MenuItem("Tools/Rogue Fairy Studio/Import approved credits")]
    public static void Credits()
    {
        Directory.CreateDirectory(Path.Combine(Application.dataPath, "StreamingAssets"));
        Run("credits", Setting("STUDIO_SNAPSHOT_ID"), Path.Combine(Application.dataPath, "StreamingAssets", "credits.json"));
    }

    [MenuItem("Tools/Rogue Fairy Studio/Publish selected asset")]
    public static void Artifact()
    {
        string asset = AssetDatabase.GetAssetPath(Selection.activeObject);
        if (!File.Exists(asset)) { EditorUtility.DisplayDialog("Studio", "Select a saved asset file.", "OK"); return; }
        Run("artifact", Path.GetFullPath(asset), "--artifact-id", Setting("STUDIO_ARTIFACT_ID"), "--source", "unity");
    }

    [MenuItem("Tools/Rogue Fairy Studio/Publish build")]
    public static void Build()
    {
        Run("build", Setting("STUDIO_BUILD_PATH"), "--source", "unity", "--release-id", Setting("STUDIO_RELEASE_ID"), "--snapshot-id", Setting("STUDIO_SNAPSHOT_ID"), "--name", Setting("STUDIO_BUILD_NAME"));
    }
}
