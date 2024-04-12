import json
import sys

if len(sys.argv) > 1:
    filename = sys.argv[1]
else:
    print("Give file as an argument")
    exit()

data = json.load(open(filename, 'r'))

for i in range(len(data['player_scores'])):
    print(f"{data['player_scores'][i]['nickname']} all kills {data['player_scores'][i]['kills']}")
    for ktype in data['player_scores'][i]['kills_by_type'].keys():
        killtype = 'kills'
        
        for k in data['kd_type_bits'].keys():
            if bool(int(ktype) & 2**int(k)):
                killtype = killtype + ' ' + data['kd_type_bits'][k]
            
        print(f"{killtype} {data['player_scores'][i]['kills_by_type'][ktype]}", end='\n')
    print()