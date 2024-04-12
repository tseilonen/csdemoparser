import json

data = json.load(open('data/parsed/tomp.dem_scoreboard.json', 'r'))

for i in range(len(data['player_scores'])):
    print(f"{data['player_scores'][i]['nickname']} all kills {data['player_scores'][i]['kills']}")
    for ktype in data['player_scores'][i]['kills_by_type'].keys():
        killtype = 'kills'
        
        for k in data['kd_type_bits'].keys():
            if bool(int(ktype) & 2**int(k)):
                killtype = killtype + ' ' + data['kd_type_bits'][k]
            
        print(f"{killtype} {data['player_scores'][i]['kills_by_type'][ktype]}", end='\n')
    print()