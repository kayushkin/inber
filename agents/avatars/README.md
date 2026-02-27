# Míl Adventurer Avatars

This directory contains pixel art avatars for the Míl adventurers.

## Characters

- **fionn.png** — Fionn the Scholar (Scribe/Wizard)
- **scathach.png** — Scáthach the Sentinel (Guardian/Warrior)
- **oisin.png** — Oisín the Courier (Ranger/Courier)
- **bran.png** — Bran the Strategist (Commander/Strategist)

## Generating Avatars

To generate the pixel art avatars:

```bash
export OPENAI_API_KEY="your-key-here"
./generate_avatars.sh
```

The script uses DALL-E 3 to create 64x64-style pixel art rendered at high resolution (1024x1024).

## Style Guide

- **Pixel art aesthetic** — Classic JRPG / retro RPG style
- **Front-facing portraits** — Character looking at camera
- **Simple backgrounds** — Focus on the character
- **Color schemes** — Each character has their own palette
  - Fionn: Purple/blue (scholarly)
  - Scáthach: Silver/red (protective)
  - Oisín: Green/brown/gold (nature, adventure)
  - Bran: Blue/gold (leadership, strategy)

## Alternative: Manual Creation

If you prefer to create avatars manually or use a different tool:
- Target size: 64x64 pixels (can be scaled up)
- Format: PNG with transparency
- Style: Fantasy RPG, pixel art
- Match character descriptions in their .md files
