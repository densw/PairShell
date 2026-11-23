using System;
using System.IO;
using System.Diagnostics;
using System.Windows.Forms;
using System.Reflection;
[assembly: AssemblyVersion("1.1.0.0")]
[assembly: AssemblyFileVersion("1.1.0.0")]
internal static class RoleLauncher {
 [STAThread] static void Main(string[] args) {
  string root=AppDomain.CurrentDomain.BaseDirectory;
  string core=Path.Combine(root,"runtime","PairShell.exe");
  string name=Path.GetFileNameWithoutExtension(Application.ExecutablePath);
  string flags=name.IndexOf("Merchant",StringComparison.OrdinalIgnoreCase)>=0?"--merchant":"--customer";
  foreach(string arg in args)if(arg=="--smoke-test"||arg=="--layout-test")flags+=" "+arg;
  try {
   if(!File.Exists(core))throw new FileNotFoundException();
   Process.Start(new ProcessStartInfo(core,flags){UseShellExecute=true,WorkingDirectory=root});
  } catch {MessageBox.Show("无法启动 PairShell。请重新安装，或确认 runtime 子目录与启动程序保存在一起。","PairShell",MessageBoxButtons.OK,MessageBoxIcon.Error);}
 }
}
