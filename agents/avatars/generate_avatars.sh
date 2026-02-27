#!/bin/bash
# Generate pixel art avatars for Míl adventurers using OpenAI DALL-E API

set -e

API_KEY="${OPENAI_API_KEY}"
if [ -z "$API_KEY" ]; then
    echo "Error: OPENAI_API_KEY not set"
    exit 1
fi

echo "Generating Míl Adventurer avatars..."

# Function to generate and download an avatar
generate_avatar() {
    local name=$1
    local prompt=$2
    local output=$3
    
    echo "Generating ${name}..."
    
    # Call OpenAI DALL-E API
    response=$(curl -s https://api.openai.com/v1/images/generations \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${API_KEY}" \
        -d "{
            \"model\": \"dall-e-3\",
            \"prompt\": \"${prompt}\",
            \"n\": 1,
            \"size\": \"1024x1024\",
            \"quality\": \"standard\"
        }")
    
    # Extract URL
    url=$(echo "$response" | grep -o '"url":"[^"]*' | cut -d'"' -f4)
    
    if [ -z "$url" ]; then
        echo "Error generating ${name}"
        echo "$response"
        return 1
    fi
    
    # Download image
    curl -s "$url" -o "$output"
    echo "✓ ${name} saved to ${output}"
}

# Fionn the Scholar - Scribe/Wizard with scrolls
generate_avatar "Fionn" \
    "Pixel art character portrait, 64x64 style but rendered at high resolution, fantasy RPG aesthetic. A scholarly wizard scribe with robes, holding ancient scrolls and a glowing quill. Calm, focused expression. Purple and blue color scheme. Clean pixel art style like classic JRPGs. Front-facing portrait, simple background." \
    "fionn.png"

# Scáthach the Sentinel - Armored Guardian with shield
generate_avatar "Scáthach" \
    "Pixel art character portrait, 64x64 style but rendered at high resolution, fantasy RPG aesthetic. A vigilant warrior woman in polished armor, holding a large shield and spear. Alert, protective stance. Silver and red color scheme. Strong, defensive posture. Clean pixel art style like classic JRPGs. Front-facing portrait, simple background." \
    "scathach.png"

# Oisín the Courier - Ranger with pack and bow
generate_avatar "Oisín" \
    "Pixel art character portrait, 64x64 style but rendered at high resolution, fantasy RPG aesthetic. A swift ranger courier with a traveling pack, bow on back, confident energetic pose. Ready to run. Green and brown color scheme with gold accents. Clean pixel art style like classic JRPGs. Front-facing portrait, simple background." \
    "oisin.png"

# Bran the Strategist - Commander with map and banner
generate_avatar "Bran" \
    "Pixel art character portrait, 64x64 style but rendered at high resolution, fantasy RPG aesthetic. A wise commander strategist holding a map or battle plans, wearing a cloak and commander's insignia. Calm, thoughtful expression. Deep blue and gold color scheme. Leadership presence. Clean pixel art style like classic JRPGs. Front-facing portrait, simple background." \
    "bran.png"

echo ""
echo "All avatars generated successfully!"
echo "Find them in: $(pwd)"
