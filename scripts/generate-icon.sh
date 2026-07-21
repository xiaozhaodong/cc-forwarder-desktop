#!/bin/bash
# 生成 macOS 风格的 AI-Switchboard 应用图标（彩色版本）

OUTPUT="build/appicon.png"

# 使用 ImageMagick 直接绘制
magick -size 1024x1024 xc:none \
  \( -size 1024x1024 xc:none -draw "roundrectangle 0,0 1024,1024 229,229" \) \
  \( -size 1024x1024 gradient:'#667eea-#764ba2' -draw "roundrectangle 0,0 1024,1024 229,229" \) \
  -compose Over -composite \
  \( -size 1024x1024 xc:none \
     -fill white -stroke none \
     -draw "circle 232,512 277,512" \
     -draw "circle 232,512 262,512" \
     -fill 'rgba(255,255,255,0.2)' \
     -draw "roundrectangle 432,432 592,592 30,30" \
     -fill 'rgba(255,255,255,0.95)' \
     -draw "roundrectangle 452,452 572,572 20,20" \
     -fill '#667eea' \
     -draw "polygon 482,492 542,492 542,462 592,512 542,562 542,532 482,530" \
     -fill 'rgba(255,255,255,0.5)' \
     -draw "circle 792,432 827,432" \
     -draw "circle 792,512 827,512" \
     -draw "circle 792,592 827,592" \
     -fill 'rgba(255,255,255,0.6)' \
     -draw "line 277,512 432,512" \
     -draw "line 572,472 757,432" \
     -draw "line 572,512 757,512" \
     -draw "line 572,552 757,592" \
  \) -compose Over -composite \
  \( -size 1024x1024 radial-gradient:'rgba(255,255,255,0.15)-rgba(255,255,255,0)' \) \
  -compose Over -composite \
  "$OUTPUT"

echo "✅ macOS 风格图标生成完成: $OUTPUT (1024x1024)"
ls -lh "$OUTPUT"
