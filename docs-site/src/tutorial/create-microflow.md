# Creating a Microflow

Microflows are the server-side logic of a Mendix application. They're comparable to functions or methods in traditional programming. In this page you'll create a microflow that accepts parameters, creates an object, commits it to the database, and returns it.

## Create a simple microflow

This microflow takes a name and price, creates a `Product` object, commits it, and returns it:

```sql
CREATE MICROFLOW MyModule.CreateProduct(
    DECLARE $Name: String,
    DECLARE $Price: Decimal
)
RETURN MyModule.Product
BEGIN
    CREATE $Product: MyModule.Product (
        Name = $Name,
        Price = $Price,
        IsActive = true
    );
    COMMIT $Product;
    RETURN $Product;
END;
```

Let's walk through each part:

| Part | Meaning |
|------|---------|
| `MyModule.CreateProduct` | Fully qualified microflow name |
| `DECLARE $Name: String` | Input parameter -- a string value passed by the caller |
| `RETURN MyModule.Product` | The microflow returns a `Product` entity object |
| `BEGIN ... END` | The microflow body |
| `CREATE $Product: MyModule.Product (...)` | Creates a new `Product` object in memory and sets its attributes |
| `COMMIT $Product` | Persists the object to the database |
| `RETURN $Product` | Returns the committed object to the caller |

Save this to a file (e.g., `create-product-mf.mdl`) and execute:

```bash
mxcli check create-product-mf.mdl
mxcli exec create-product-mf.mdl -p app.mpr
```

## Verify the microflow

Use `DESCRIBE MICROFLOW` to see the generated MDL:

```bash
mxcli -p app.mpr -c "DESCRIBE MICROFLOW MyModule.CreateProduct"
```

This shows the full microflow definition, including parameters, activities, and the return type.

## Understanding variables

All variables in MDL start with `$`. There are two contexts where you declare them:

**Parameters** (in the signature):

```sql
CREATE MICROFLOW MyModule.DoSomething(
    DECLARE $Customer: MyModule.Customer,
    DECLARE $Count: Integer
)
```

**Local variables** (inside BEGIN...END):

```sql
BEGIN
    DECLARE $Total: Integer = 0;
    DECLARE $Message: String = 'Processing...';
    DECLARE $Items: List of MyModule.Item = empty;
END;
```

For entity parameters, do not assign a default value -- just declare the type:

```sql
-- Correct: entity parameter with no default
DECLARE $Customer: MyModule.Customer

-- Wrong: entity parameter with = empty
DECLARE $Customer: MyModule.Customer = empty
```

## Retrieving objects

Use `RETRIEVE` to fetch objects from the database:

```sql
CREATE MICROFLOW MyModule.GetActiveProducts()
RETURN List of MyModule.Product
BEGIN
    RETRIEVE $Products: List of MyModule.Product
        FROM MyModule.Product
        WHERE IsActive = true;
    RETURN $Products;
END;
```

To retrieve a single object, add `LIMIT 1`:

```sql
RETRIEVE $Product: MyModule.Product
    FROM MyModule.Product
    WHERE Name = $SearchName
    LIMIT 1;
```

## Conditional logic

Use `IF ... THEN ... ELSE ... END IF` for branching:

```sql
CREATE MICROFLOW MyModule.UpdateProductStatus(
    DECLARE $Product: MyModule.Product
)
RETURN Boolean
BEGIN
    IF $Product/Price > 0 THEN
        CHANGE $Product (IsActive = true);
        COMMIT $Product;
        RETURN true;
    ELSE
        LOG WARNING 'Product has no price set';
        RETURN false;
    END IF;
END;
```

## Looping over a list

Use `LOOP ... IN ... BEGIN ... END LOOP` to iterate:

```sql
CREATE MICROFLOW MyModule.DeactivateProducts(
    DECLARE $Products: List of MyModule.Product
)
RETURN Boolean
BEGIN
    LOOP $Product IN $Products
    BEGIN
        CHANGE $Product (IsActive = false);
        COMMIT $Product;
    END LOOP;
    RETURN true;
END;
```

Note: the list `$Products` is a parameter. Never create an empty list variable and then loop over it -- accept the list as a parameter instead.

## Error handling

Add `ON ERROR` to handle failures on individual activities:

```sql
COMMIT $Product ON ERROR CONTINUE;
```

Options are `CONTINUE` (ignore the error and proceed), `ROLLBACK` (roll back the transaction and continue), or an inline error handler block:

```sql
COMMIT $Product ON ERROR BEGIN
    LOG ERROR 'Failed to commit product';
    RETURN false;
END ERROR;
```

## Organizing with folders

Place microflows in folders using the `FOLDER` keyword:

```sql
CREATE MICROFLOW MyModule.CreateProduct(
    DECLARE $Name: String,
    DECLARE $Price: Decimal
)
RETURN MyModule.Product
FOLDER 'ACT'
BEGIN
    CREATE $Product: MyModule.Product (
        Name = $Name,
        Price = $Price,
        IsActive = true
    );
    COMMIT $Product;
    RETURN $Product;
END;
```

This places the microflow in the `ACT` folder within the module, following the common Mendix convention of grouping microflows by type (ACT for actions, VAL for validations, SUB for sub-microflows).

## Common mistakes

**Every flow path must end with RETURN.** If your microflow has `IF/ELSE` branches, each branch needs its own `RETURN` statement (or the return can come after `END IF` if both branches converge).

**COMMIT is required to persist changes.** `CREATE` and `CHANGE` only modify the object in memory. Without `COMMIT`, changes are lost when the microflow ends.

**Entity parameters don't use `= empty`.** Declare them with just the type. Assigning `= empty` is for list variables inside the microflow body, not for parameters.
