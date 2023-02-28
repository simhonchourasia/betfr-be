# betfr-be
API for betting app with friends

## Setup

#### Config file
Create a file named `default.json` in the `config` folder, with the following structure: 
```json
{
    "mongoURI": "PUT_MONGO_URI_HERE",
    "cluster": "cluster0",
    "userCollection": "Users",
    "betCollection": "Bets",
    "stakeCollection": "Stakes",
    "secretKey": "SECRET_KEY_GOES_HERE",
    "port": "8000"
}
```

