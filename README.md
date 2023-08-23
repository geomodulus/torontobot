# TorontoBot

TorontoBot is a tool for querying Toronto Open Data. It answers questions either on the command line
or as a Discord bot.

To help explain, here are some [slides](https://docs.google.com/presentation/d/18zs_1IhCaF1aJ-cQCWIBr0Ga2Zk6f17XL1xXPsy54yo).

**We're looking for a Go developers interested in working with LLMs to help us turn TorontoBot into
a richly-featured, _civic assistant_.**

## Usage

Join [our Discord](https://discord.gg/sggsjGet3E). In the "Open Data" channel, using a slash command:

    /torontobot <your-query-here>

This bot is brand new, so go easy on it if it doesn't get things right. It can answer questions
about the operating budget surprisingly well!

To run queries locally you'll need an OpenAI API token. Get one at openai.com.

As of right now, TorontoBot can answer questions about:

  - The City of Toronto operating budget
  - 311 service requests
  - Speed camera ticket information
  - StatsCan apartment price index.

There are plenty of examples in th Discord. Don't hesitate to ask TorontoBot how it can help!

## Dependencies

This project requires the Go programming language and sqlite3 installed on your local system. Both
very easy to install.

## Ingest

First thing to do is initialize the database. We're building our proof-of-concept with a single
data source: the City of Toronto approved operating budget. This file has good quality from 2014 
and because it's one big flat table covering the whole city budget, it's useful for a lot of
different queries.

We need to create a local database file containing the target data so we can query it locally. Bear
in mind, if you want to include 311 request data you are in for a wait.

This project uses [golang-migrate/migrate](https://github.com/golang-migrate/migrate) to manage
migrations and you can install it like so:

```
 $~/code/torontobot/db> go  install -tags 'sqlite3' github.com/golang-migrate/migrate/v4/cmd/migrate
```

That will add a `migrate` binary to your Go binary path.

Next, you'll need to intialize a fresh, empty database. Do it like this (in the `db` dir):

```
 $~/code/torontobot/db> migrate -path db/migrations -database "sqlite3://db/toronto.db" up 
```

That will create `toronto.db`, the `sqlite3` database we'll use to store our data.

Then, from the ingest directory, run the ingest script:

```
 $~/code/torontobot/ingest> go run .
```

Over the course of the next several minutes, this script will download City of Toronto operating
budget data for the years 2014 through 2023 collating and storing every entry in our database file.

Then, over several more hours it will load each of the 311 service request files for the last ~13
years and write those to the database as well.

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

## Adding a new dataset

There are three steps required to add a new dataset.

1. Figure out what schema to use. (easy)
2. Write ingest script for the dataset. (hard)
3. Add table description to [tables.json5](https://github.com/geomodulus/torontobot/blob/main/bot/tables.json5) (very easy)

Don't hesitate to give a sample of the data to GPT-4 and ask for a draft schema.

The hard part is figuring out how to ingest the data. There are two examples in the current
[ingest script](https://github.com/geomodulus/torontobot/blob/main/ingest/main.go), but every piece
of data is unique and it's important to give the dataset proper consideration during ingest.

`tables.json5` will let you add both hints and special instructions for the table. This can improve
the query experience dramatically.

## Inspiration

This project is inspired by the work being done on [textSQL](https://github.com/caesarHQ/textSQL),
we've retooled the idea to better fit the Torontoverse ecosystem.

## Support

Join [our Discord](https://discord.gg/sggsjGet3E).
