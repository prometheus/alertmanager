module Debugger.Report exposing
  ( Report(..)
  , Change(..)
  , TagChanges
  , emptyTagChanges
  , hasTagChanges
  , Status(..), evaluate
  )



-- REPORTS


type Report
  = CorruptHistory
  | VersionChanged String String
  | MessageChanged String String
  | SomethingChanged (List Change)


type Change
  = AliasChange String
  | UnionChange String TagChanges


type alias TagChanges =
  { removed : List String
  , changed : List String
  , added : List String
  , argsMatch : Bool
  }


emptyTagChanges : Bool -> TagChanges
emptyTagChanges argsMatch =
  TagChanges [] [] [] argsMatch


hasTagChanges : TagChanges -> Bool
hasTagChanges tagChanges =
  tagChanges == TagChanges [] [] [] True


type Status = Impossible | Risky | Fine


evaluate : Report -> Status
evaluate report =
  case report of
    CorruptHistory ->
      Impossible

    VersionChanged _ _ ->
      Impossible

    MessageChanged _ _ ->
      Impossible

    SomethingChanged changes ->
      worstCase Fine (List.map evaluateChange changes)


worstCase : Status -> List Status -> Status
worstCase status statusList =
  case statusList of
    [] ->
      status

    Impossible :: _ ->
      Impossible

    Risky :: rest ->
      worstCase Risky rest

    Fine :: rest ->
      worstCase status rest


evaluateChange : Change -> Status
evaluateChange change =
  case change of
    AliasChange _ ->
      Impossible

    UnionChange _ { removed, changed, added, argsMatch } ->
      if not argsMatch || some changed || some removed then
        Impossible

      else if some added then
        Risky

      else
        Fine


some : List a -> Bool
some list =
  not (List.isEmpty list)
