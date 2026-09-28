/**
 * MDL Security Grammar — security statements (module roles, user roles,
 * grants, revokes, project security, demo users).
 */
parser grammar MDLSecurity;

options { tokenVocab = MDLLexer; }

// =============================================================================
// SECURITY STATEMENTS
// =============================================================================

// A createStatement kind (MDLParser.g4), so it takes the same prefixes as every
// other document type: OR MODIFY makes a security script re-runnable (without
// it, re-executing the script that sets up roles fails on the first role that
// already exists), and a doc comment attaches to it (#731).
createModuleRoleStatement
    : MODULE ROLE ifNotExists? qualifiedName (DESCRIPTION STRING_LITERAL)?
    ;

dropModuleRoleStatement
    : DROP MODULE ROLE ifExists? qualifiedName
    ;

// A user role's properties are a ( Key: value ) list, as a scheduled event's
// are (ako/mxcli#707, PROPOSAL_mdl_beta_syntax_freeze.md §4 Security). The
// positional form had no slot for Description or CheckSecurity, so describe
// printed them as comments and a replay lost them, and it required at least one
// module role, so a role with none described into a statement that did not
// parse. The list is optional, and may be empty: `create user role Guest;`.
// The positional form is the deprecated alias; both start `( <name>`, and the
// `:` after the first name tells them apart.
createUserRoleStatement
    : USER ROLE ifNotExists? identifierOrKeyword userRolePropertyList?
    | USER ROLE ifNotExists? identifierOrKeyword /* @alias MDL-DEPR710 */
      LPAREN moduleRoleList RPAREN
      (MANAGE ALL ROLES)?
    ;

userRolePropertyList
    : LPAREN (userRoleProperty (COMMA userRoleProperty)* COMMA?)? RPAREN
    ;

// ModuleRoles: (M.R, …) and ManageableRoles: (UserRole, …) take a list;
// Description a string; ManageAllRoles, ManageUsersWithoutRoles and
// CheckSecurity a boolean. The visitor refuses any other key.
userRoleProperty
    : identifierOrKeyword COLON LPAREN (qualifiedName (COMMA qualifiedName)*)? RPAREN
    | identifierOrKeyword COLON STRING_LITERAL
    | identifierOrKeyword COLON booleanLiteral
    ;

alterUserRoleStatement
    : ALTER USER ROLE identifierOrKeyword ADD MODULE ROLES LPAREN moduleRoleList RPAREN
    // R6: an alter's children are added and dropped; `remove` is the old verb.
    | ALTER USER ROLE identifierOrKeyword (DROP | REMOVE /* @alias MDL-DEPR091 */) MODULE ROLES LPAREN moduleRoleList RPAREN
    ;

// IF EXISTS makes a cleanup script re-runnable. Without it the statement fails
// the second time, so a one-time cleanup either breaks every later run of the
// slice or has to be commented out — which is what happened to
// `drop demo user` / `drop user role` in a real project (ako/CapTrackV4 R5).
// Same spelling as ALTER ENTITY's DROP ATTRIBUTE IF EXISTS.
dropUserRoleStatement
    : DROP USER ROLE ifExists? (identifierOrKeyword | STRING_LITERAL)
    ;

// The canonical form names the rights first and the roles after TO, like every
// other grant, and takes the XPath in [ ] as every other XPath is written (R5,
// ako/mxcli#753): Mendix stores sibling predicate groups concatenated, so a
// constraint is one or more groups. The reversed form, with the XPath in a
// string, is the deprecated alias. The two start differently after GRANT (a
// right keyword and `*`/`(`, or a role name and ON), so both parse.
grantEntityAccessStatement
    : GRANT entityAccessRightList ON ENTITY qualifiedName TO moduleRoleList
      (WHERE xpathConstraint+)?
    | GRANT moduleRoleList ON qualifiedName /* @alias MDL-DEPR030 */
      LPAREN entityAccessRightList RPAREN
      (WHERE STRING_LITERAL)?
    ;

// R5 (ako/mxcli#753): the revoke mirrors the grant — rights first, then
// `on entity`, the roles after `from`. `all` removes the roles' access rule
// altogether; a rights list takes those rights away and keeps the rule.
revokeEntityAccessStatement
    : REVOKE (ALL | entityAccessRightList) ON ENTITY qualifiedName FROM moduleRoleList
    | REVOKE moduleRoleList ON qualifiedName /* @alias MDL-DEPR082 */
      (LPAREN entityAccessRightList RPAREN)?
    ;

grantMicroflowAccessStatement
    : GRANT EXECUTE ON MICROFLOW qualifiedName TO moduleRoleList
    ;

revokeMicroflowAccessStatement
    : REVOKE EXECUTE ON MICROFLOW qualifiedName FROM moduleRoleList
    ;

grantNanoflowAccessStatement
    : GRANT EXECUTE ON NANOFLOW qualifiedName TO moduleRoleList
    ;

revokeNanoflowAccessStatement
    : REVOKE EXECUTE ON NANOFLOW qualifiedName FROM moduleRoleList
    ;

grantPageAccessStatement
    : GRANT VIEW ON PAGE qualifiedName TO moduleRoleList
    ;

revokePageAccessStatement
    : REVOKE VIEW ON PAGE qualifiedName FROM moduleRoleList
    ;

// There is no `grant|revoke execute on workflow`: a Mendix workflow has no
// allowed roles of its own — who may start one is the microflow that calls it,
// and who may act on it is a user task's targeting. The form parsed and exec
// always refused it; it was removed as dead grammar (ako/mxcli#756), and the
// parse error says where the access lives instead.

grantODataServiceAccessStatement
    : GRANT ACCESS ON publishedODataServiceKw qualifiedName TO moduleRoleList
    ;

revokeODataServiceAccessStatement
    : REVOKE ACCESS ON publishedODataServiceKw qualifiedName FROM moduleRoleList
    ;

grantPublishedRestServiceAccessStatement
    : GRANT ACCESS ON PUBLISHED REST SERVICE qualifiedName TO moduleRoleList
    ;

revokePublishedRestServiceAccessStatement
    : REVOKE ACCESS ON PUBLISHED REST SERVICE qualifiedName FROM moduleRoleList
    ;

alterProjectSecurityStatement
    // R10 (ako/mxcli#755): `alter app security ( Key: value, … )`, create's
    // property list with Security$ProjectSecurity's property names:
    // SecurityLevel, EnableDemoUsers, EnableGuestAccess, GuestUserRole,
    // StrictMode. Each clause form below is a deprecated alias of one key.
    : ALTER appSecurityKw settingsItemOptions
    | ALTER appSecurityKw LEVEL (PRODUCTION | PROTOTYPE | OFF) /* @alias MDL-DEPR133 */
    | ALTER appSecurityKw DEMO USERS (ON | OFF) /* @alias MDL-DEPR133 */
    // ROLE is optional here but effectively required by Mendix: mxbuild raises
    // CE0133 when guest access is on with no role. It is optional so that
    // re-enabling a project that already stores one does not force a retype;
    // the executor refuses ON when neither source supplies a role.
    | ALTER appSecurityKw GUEST ACCESS ON (ROLE identifierOrKeyword)? /* @alias MDL-DEPR133 */
    | ALTER appSecurityKw GUEST ACCESS OFF /* @alias MDL-DEPR133 */
    // Strict mode is a plain bool on Security$ProjectSecurity, declared by BOTH
    // generated sources and already read back from real projects — so this
    // writes a property Studio Pro knows, not one gen merely offers.
    //
    // mxcli LINTED for it (SEC005) and offered no way to clear it, which is a
    // rule with no remedy (ako/mxcli#526).
    | ALTER appSecurityKw STRICT MODE (ON | OFF) /* @alias MDL-DEPR133 */
    ;

// R10: Studio Pro calls it App Security; `project security` is the old name.
appSecurityKw
    : APP SECURITY
    | PROJECT SECURITY /* @alias MDL-DEPR554 */
    ;

createDemoUserStatement
    : DEMO USER ifNotExists? STRING_LITERAL PASSWORD STRING_LITERAL (ENTITY qualifiedName)?
      LPAREN identifierOrKeyword (COMMA identifierOrKeyword)* RPAREN
    ;

dropDemoUserStatement
    : DROP DEMO USER ifExists? STRING_LITERAL
    ;

// IN is optional before the module name, not just before the whole clause.
// `update security RestLab` used to reach the parser's error recovery, which
// consumed the name silently: the statement parsed as ONE statement with no
// error, `mxcli check` reported "Syntax OK", and the run went project-wide.
// A scope the author asked for and did not get is worse than a parse error.
// (mendixlabs/mxcli#1047)
updateSecurityStatement
    : UPDATE SECURITY (IN? qualifiedName)?
    ;

moduleRoleList
    : qualifiedName (COMMA qualifiedName)*
    ;

entityAccessRightList
    : entityAccessRight (COMMA entityAccessRight)*
    ;

entityAccessRight
    : CREATE
    | DELETE
    | READ STAR
    | READ LPAREN entityMemberName (COMMA entityMemberName)* RPAREN
    | WRITE STAR
    | WRITE LPAREN entityMemberName (COMMA entityMemberName)* RPAREN
    ;

// Member (attribute / association) name in a READ/WRITE list. Accepts a quoted
// identifier so members whose name is a reserved word can be escaped, e.g.
// READ ("Order", Status).
entityMemberName
    : IDENTIFIER
    | QUOTED_IDENTIFIER
    ;
