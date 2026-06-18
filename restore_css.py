import json
import re

transcript_path = '/Users/eduardgonzalez/.gemini/antigravity/brain/2a8d5c94-e15e-411d-9061-537fa52790be/.system_generated/logs/transcript_full.jsonl'
css_lines = {}

with open(transcript_path, 'r') as f:
    for line in f:
        try:
            entry = json.loads(line)
            content = entry.get('content', '')
            if 'styles.css' in content and 'Total Lines: 1984' in content:
                # We know this is one of our early view_file responses
                # It has lines formatted as "number: text\n" or embedded in a JSON string.
                # Since content is a python string, we can split it by \n
                lines = content.split('\n')
                for l in lines:
                    match = re.match(r'^(\d+): (.*)$', l)
                    if match:
                        line_num = int(match.group(1))
                        line_text = match.group(2)
                        # We only keep the FIRST time we see a line to avoid getting the ones from later viewings if they existed (though all 1984 lines were viewed once)
                        if line_num not in css_lines:
                            css_lines[line_num] = line_text
        except Exception as e:
            pass

if css_lines:
    max_line = max(css_lines.keys())
    with open('/Users/eduardgonzalez/Documents/03-Proyectos/upgopher/internal/statics/css/styles.css', 'w') as out_f:
        for i in range(1, max_line + 1):
            out_f.write(css_lines.get(i, '') + '\n')
    print(f"Restored {max_line} lines.")
else:
    print("No lines found.")
