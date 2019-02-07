module Debugger.Metadata exposing
  ( Metadata
  , check
  , decode, decoder, encode
  , Error, ProblemType, Problem(..)
  )


import Array exposing (Array)
import Dict exposing (Dict)
import Json.Decode as Decode
import Json.Encode as Encode
import Debugger.Report as Report exposing (Report)



-- METADATA


type alias Metadata =
  { versions : Versions
  , types : Types
  }



-- VERSIONS


type alias Versions =
  { elm : String
  }



-- TYPES


type alias Types =
  { message : String
  , aliases : Dict String Alias
  , unions : Dict String Union
  }


type alias Alias =
  { args : List String
  , tipe : String
  }


type alias Union =
  { args : List String
  , tags : Dict String (List String)
  }



-- PORTABILITY


isPortable : Metadata -> Maybe Error
isPortable {types} =
  let
    badAliases =
      Dict.foldl collectBadAliases [] types.aliases
  in
    case Dict.foldl collectBadUnions badAliases types.unions of
      [] ->
        Nothing

      problems ->
        Just (Error types.message problems)


type alias Error =
  { message : String
  , problems : List ProblemType
  }


type alias ProblemType =
  { name : String
  , problems : List Problem
  }


type Problem
  = Function
  | Decoder
  | Task
  | Process
  | Socket
  | Request
  | Program
  | VirtualDom


collectBadAliases : String -> Alias -> List ProblemType -> List ProblemType
collectBadAliases name {tipe} list =
  case findProblems tipe of
    [] ->
      list

    problems ->
      ProblemType name problems :: list


collectBadUnions : String -> Union -> List ProblemType -> List ProblemType
collectBadUnions name {tags} list =
  case List.concatMap findProblems (List.concat (Dict.values tags)) of
    [] ->
      list

    problems ->
      ProblemType name problems :: list


findProblems : String -> List Problem
findProblems tipe =
  List.filterMap (hasProblem tipe) problemTable


hasProblem : String -> (Problem, String) -> Maybe Problem
hasProblem tipe (problem, token) =
  if String.contains token tipe then Just problem else Nothing


problemTable : List (Problem, String)
problemTable =
  [ ( Function, "->" )
  , ( Decoder, "Json.Decode.Decoder" )
  , ( Task, "Task.Task" )
  , ( Process, "Process.Id" )
  , ( Socket, "WebSocket.LowLevel.WebSocket" )
  , ( Request, "Http.Request" )
  , ( Program, "Platform.Program" )
  , ( VirtualDom, "VirtualDom.Node" )
  , ( VirtualDom, "VirtualDom.Attribute" )
  ]



-- CHECK


check : Metadata -> Metadata -> Report
check old new =
  if old.versions.elm /= new.versions.elm then
    Report.VersionChanged old.versions.elm new.versions.elm

  else
    checkTypes old.types new.types


checkTypes : Types -> Types -> Report
checkTypes old new =
  if old.message /= new.message then
    Report.MessageChanged old.message new.message

  else
    []
      |> Dict.merge ignore checkAlias ignore old.aliases new.aliases
      |> Dict.merge ignore checkUnion ignore old.unions new.unions
      |> Report.SomethingChanged


ignore : String -> value -> a -> a
ignore key value report =
  report



-- CHECK ALIASES


checkAlias : String -> Alias -> Alias -> List Report.Change -> List Report.Change
checkAlias name old new changes =
  if old.tipe == new.tipe && old.args == new.args then
    changes

  else
    Report.AliasChange name :: changes



-- CHECK UNIONS


checkUnion : String -> Union -> Union -> List Report.Change -> List Report.Change
checkUnion name old new changes =
  let
    tagChanges =
      Dict.merge removeTag checkTag addTag old.tags new.tags <|
        Report.emptyTagChanges (old.args == new.args)
  in
    if Report.hasTagChanges tagChanges then
      changes

    else
      Report.UnionChange name tagChanges :: changes


removeTag : String -> a -> Report.TagChanges -> Report.TagChanges
removeTag tag _ changes =
  { changes | removed = tag :: changes.removed }


addTag : String -> a -> Report.TagChanges -> Report.TagChanges
addTag tag _ changes =
  { changes | added = tag :: changes.added }


checkTag : String -> a -> a -> Report.TagChanges -> Report.TagChanges
checkTag tag old new changes =
  if old == new then
    changes

  else
    { changes | changed = tag :: changes.changed }



-- JSON DECODE


decode : Encode.Value -> Result Error Metadata
decode value =
  case Decode.decodeValue decoder value of
    Err _ ->
      Err (Error "The compiler is generating bad metadata. This is a compiler bug!" [])

    Ok metadata ->
      case isPortable metadata of
        Nothing ->
          Ok metadata

        Just error ->
          Err error


decoder : Decode.Decoder Metadata
decoder =
  Decode.map2 Metadata
    (Decode.field "versions" decodeVersions)
    (Decode.field "types" decodeTypes)


decodeVersions : Decode.Decoder Versions
decodeVersions =
  Decode.map Versions
    (Decode.field "elm" Decode.string)


decodeTypes : Decode.Decoder Types
decodeTypes =
  Decode.map3 Types
    (Decode.field "message" Decode.string)
    (Decode.field "aliases" (Decode.dict decodeAlias))
    (Decode.field "unions" (Decode.dict decodeUnion))


decodeUnion : Decode.Decoder Union
decodeUnion =
  Decode.map2 Union
    (Decode.field "args" (Decode.list Decode.string))
    (Decode.field "tags" (Decode.dict (Decode.list Decode.string)))


decodeAlias : Decode.Decoder Alias
decodeAlias =
  Decode.map2 Alias
    (Decode.field "args" (Decode.list Decode.string))
    (Decode.field "type" (Decode.string))



-- JSON ENCODE


encode : Metadata -> Encode.Value
encode { versions, types } =
  Encode.object
    [ ("versions", encodeVersions versions)
    , ("types", encodeTypes types)
    ]


encodeVersions : Versions -> Encode.Value
encodeVersions { elm } =
  Encode.object [("elm", Encode.string elm)]


encodeTypes : Types -> Encode.Value
encodeTypes { message, unions, aliases } =
  Encode.object
    [ ("message", Encode.string message)
    , ("aliases", encodeDict encodeAlias aliases)
    , ("unions", encodeDict encodeUnion unions)
    ]


encodeAlias : Alias -> Encode.Value
encodeAlias { args, tipe } =
  Encode.object
    [ ("args", Encode.list Encode.string args)
    , ("type", Encode.string tipe)
    ]


encodeUnion : Union -> Encode.Value
encodeUnion { args, tags } =
  Encode.object
    [ ("args", Encode.list Encode.string args)
    , ("tags", encodeDict (Encode.list Encode.string) tags)
    ]


encodeDict : (a -> Encode.Value) -> Dict String a -> Encode.Value
encodeDict f dict =
  dict
    |> Dict.map (\key value -> f value)
    |> Dict.toList
    |> Encode.object


