# TorontoBot

TorontoBot is a tool for querying Toronto Open Data. One day soon it will be a discord bot. Today,
it just outputs reponses to the command line.

## Dependencies

This project requires the Go programming language and sqlite3 installed on your local system. Both
very easy to install.


## Ingest

First thing to do is initialize the database. We're building our proof-of-concept with a single
file: the 2023 City of Toronto approved operating budget. 

> We chose this file because it's one big flat table that's amenable to a lot of difference queries.

First, you'll need to intialize a database. Do it like this:
```
 $~/code/torontobot/db> go run migrate.go
```

Next, you'll need to download the 2023 budget data file in (sigh) XLSX format from 
[here](https://open.toronto.ca/dataset/budget-operating-budget-program-summary-by-expenditure-category/)
and save it somewhere. 

Then, run the following command:
```
 $~/code/torontobot/ingest> go run . --data-file <path-to-your-data-file>
```

This will take a few minutes as it's loading 20,000+ budget line items into your local database.

## Usage

Start the bot like so:
```
 $~/code/torontobot> go run . --openai-token <your-openai-token-here>
```

Now you can do something like this:
```
>> What are the 8 most expensive programs?
% sending request to openai...
Torontobot: The operating_budget table has columns for id, program, service, activity, entry_type, category, subcategory, item, year, and amount. We will be using the program and amount columns.

We will be looking at the amount column to determine program costs, and sorting by descending order to find the 8 most expensive programs.

Executing query "WITH program_costs AS (SELECT program, SUM(amount) AS total_cost FROM operating_budget WHERE entry_type = 'expense' GROUP BY program) SELECT program, total_cost FROM program_costs ORDER BY total_cost DESC LIMIT 8;"

Query result:
+-------------------------------------------+-------------------+
| PROGRAM (TEXT)                            | TOTAL_COST ()     |
+-------------------------------------------+-------------------+
| Toronto Transit Commission - Conventional | $2,237,543,963.48 |
| Toronto Police Service                    | $1,330,625,706.88 |
| Capital & Corporate Financing             | $1,204,852,706.30 |
| Toronto Employment & Social Services      | $1,153,609,587.01 |
| Children's Services                       | $1,108,471,252.28 |
| Housing Secretariat                       | $845,624,329.06   |
| Shelter, Support & Housing Administration | $707,949,472.87   |
| Non-Program Expenditures                  | $673,064,907.78   |
+-------------------------------------------+-------------------+

>>  
```

## Inspiration

This project is inspired by the work being done on [textSQL](https://github.com/caesarHQ/textSQL),
we've simply retooled it to better fit the Torontoverse/Geomodulus ecosystem.

## Support

Join [our Discord](https://discord.gg/E5f6vsNc).
