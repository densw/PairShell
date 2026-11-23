# PairShell 应用标识

原创标识采用成对蓝色/青绿色括号，围合白色终端提示符，表达两端配对与远程终端。不是第三方产品图标的改绘。无文字的方形标识用于桌面、Dock、任务栏及窗口内品牌位置。

- SVG 主文件：`assets/brand/pairshell.svg`。
- Windows 多尺寸图标：`assets/brand/pairshell.ico`，含 16–256 像素。
- Mac 图标源：`assets/brand/PairShell.iconset`，含标准 1x/2x 全套 PNG，1024 像素最大。
- PNG 导出：16、20、24、32、40、48、64、128、256、512、1024。
- `assets/brand/render_brand.py` 按同一原创几何重建图标；Pillow 仅用于作者导出，不进入客户端运行依赖。

深色底 `#151E2E`，蓝色 `#628BFF`，青绿色 `#47D8C8`，提示符 `#F0F6FF`。不以颜色区分权限或执行成功，避免把品牌色当作安全指示。

Windows 客户端工程通过 ApplicationIcon 编入 ICO。Mac 打包时从 iconset 调用系统 iconutil 生成 icns，并以 CFBundleIconFile 加入资源。窗口内使用离线内嵌 SVG，不请求 CDN。
