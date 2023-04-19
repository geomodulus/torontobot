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
>> What are the 5 most expensive programs?
% sending request to openai...
Torontobot: {
    "Schema": "The table 'operating_budget' has columns for program, service, activity, entry_type, category, subcategory, item, year, and amount.",
    "Applicability": "The 'program' column is relevant for this query as it contains the names of the programs. The 'amount' column is also relevant as it contains the expenses for each program.",
    "SQL": "WITH program_expenses AS (SELECT program, SUM(amount) AS total_expense FROM operating_budget WHERE entry_type='expense' GROUP BY program) SELECT program, total_expense FROM program_expenses ORDER BY total_expense DESC LIMIT 5;"
>> List total expenses less revenue for top 10 most net expensive programs
% sending request to openai...
Torontobot: {
    "Schema": "The operating_budget table has columns for program, service, activity, entry_type, category, subcategory, item, year, and amount. We will use program, entry_type, and amount columns for this query.",
    "Applicability": "We will be using the entry_type column to differentiate between expenses and revenue. We will be using the program column to group expenses and revenue by program. The amount column will be used to calculate the total expenses less revenue. There are no missing columns or enums for this query.",
    "SQL": "WITH program_expense AS (SELECT program, SUM(amount) AS total_expense FROM operating_budget WHERE entry_type = 'expense' GROUP BY program), program_revenue AS (SELECT program, SUM(amount) AS total_revenue FROM operating_budget WHERE entry_type = 'revenue' GROUP BY program) SELECT program_expense.program, program_expense.total_expense - COALESCE(program_revenue.total_revenue, 0) AS net_expense FROM program_expense LEFT JOIN program_revenue ON program_expense.program = program_revenue.program ORDER BY net_expense DESC LIMIT 10;"
```

## Inspiration

This project is inspired by the work being done on [textSQL](https://github.com/caesarHQ/textSQL),
we've simply retooled it to better fit the Torontoverse/Geomodulus ecosystem.
