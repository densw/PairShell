using System;
using System.IO;
using System.Diagnostics;
using System.Windows.Forms;
using System.Reflection;
[assembly: AssemblyVersion("1.1.2.0")]
[assembly: AssemblyFileVersion("1.1.2.0")]
internal static class RoleLauncher {
 internal static string DefaultFlags() {
#if MERCHANT_DEFAULT
  return "--merchant";
#elif CUSTOMER_DEFAULT
  return "--customer";
#else
  string name=Path.GetFileNameWithoutExtension(Application.ExecutablePath);
  return name.IndexOf("Merchant",StringComparison.OrdinalIgnoreCase)>=0?"--merchant":"--customer";
#endif
 }
 [STAThread] static void Main(string[] args) {
  string root=AppDomain.CurrentDomain.BaseDirectory;
  string core=Path.Combine(root,"runtime","PairShell.exe");
  string flags=DefaultFlags();
  foreach(string arg in args)if(arg=="--smoke-test"||arg=="--layout-test")flags+=" "+arg;
  try {
   if(!File.Exists(core))throw new FileNotFoundException();
   Process.Start(new ProcessStartInfo(core,flags){UseShellExecute=true,WorkingDirectory=root});
  } catch {MessageBox.Show("无法启动 PairShell。请重新安装，或确认 runtime 子目录与启动程序保存在一起。","PairShell",MessageBoxButtons.OK,MessageBoxIcon.Error);}
 }
}
