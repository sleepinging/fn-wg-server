#!/bin/bash
# Generate PNG icons for the WireGuard server app
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Generate icons using Python
python3 << 'PYEOF'
import struct, zlib, os

def create_shield_png(width, height, color_rgb=(33, 150, 243)):
    """Create a simple shield icon PNG."""
    
    def chunk(chunk_type, data):
        c = chunk_type + data
        return struct.pack('>I', len(data)) + c + struct.pack('>I', zlib.crc32(c) & 0xffffffff)
    
    # Create pixel data (RGB)
    raw = b''
    for y in range(height):
        raw += b'\x00'  # filter byte (none)
        for x in range(width):
            # Simple shield shape
            cx, cy = width / 2, height / 2
            h = height
            
            # Normalize coordinates to [0, 1]
            nx = x / width
            ny = y / height
            
            # Shield shape: pentagon-like
            inside = False
            if ny < 0.15:
                inside = True  # Top bar
            elif ny < 0.85:
                # Triangle narrowing toward bottom
                half_w = 0.45 * (1 - (ny - 0.15) / 0.7)
                if abs(nx - 0.5) < half_w:
                    inside = True
            
            if inside:
                raw += bytes(color_rgb)
            else:
                raw += b'\xf5\xf5\xf5'  # light gray background
    
    def make_pixel_data():
        return zlib.compress(raw)
    
    header = b'\x89PNG\r\n\x1a\n'
    ihdr = chunk(b'IHDR', struct.pack('>IIBBBBB', width, height, 8, 2, 0, 0, 0))
    idat = chunk(b'IDAT', make_pixel_data())
    iend = chunk(b'IEND', b'')
    
    return header + ihdr + idat + iend

base_dir = r"D:\dev\code\prj\fn\wg-server"

# Generate icons
icons = [
    ('ICON.PNG', 64, (33, 150, 243)),
    ('ICON_256.PNG', 256, (33, 150, 243)),
    ('icon_64.png', 64, (33, 150, 243)),
    ('icon_256.png', 256, (33, 150, 243)),
]

for name, size, color in icons:
    png_data = create_shield_png(size, size, color)
    
    if 'icon_' in name:
        path = os.path.join(base_dir, 'pkg', 'ui', 'images', name)
    else:
        path = os.path.join(base_dir, 'pkg', name)
    
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, 'wb') as f:
        f.write(png_data)
    print(f"Created: {path} ({size}x{size})")

print("All icons generated successfully!")
PYEOF
