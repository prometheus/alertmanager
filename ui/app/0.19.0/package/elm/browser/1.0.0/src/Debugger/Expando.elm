module Debugger.Expando exposing
  ( Expando
  , init
  , merge
  , Msg, update
  , view
  )


import Dict exposing (Dict)
import Elm.Kernel.Debugger
import Json.Decode as Json
import Html exposing (Html, text, div, span)
import Html.Attributes exposing (style, class)
import Html.Events exposing (onClick)



-- MODEL


type Expando
  = S String
  | Primitive String
  | Sequence SeqType Bool (List Expando)
  | Dictionary Bool (List (Expando, Expando))
  | Record Bool (Dict String Expando)
  | Constructor (Maybe String) Bool (List Expando)


type SeqType = ListSeq | SetSeq | ArraySeq


seqTypeToString : Int -> SeqType -> String
seqTypeToString n seqType =
  case seqType of
    ListSeq ->
      "List(" ++ String.fromInt n ++ ")"

    SetSeq ->
      "Set(" ++ String.fromInt n ++ ")"

    ArraySeq ->
      "Array(" ++ String.fromInt n ++ ")"



-- INITIALIZE


init : a -> Expando
init value =
  initHelp True (Elm.Kernel.Debugger.init value)


initHelp : Bool -> Expando -> Expando
initHelp isOuter expando =
  case expando of
    S _ ->
      expando

    Primitive _ ->
      expando

    Sequence seqType isClosed items ->
      if isOuter then
        Sequence seqType False (List.map (initHelp False) items)
      else if List.length items <= 8 then
        Sequence seqType False items
      else
        expando

    Dictionary isClosed keyValuePairs ->
      if isOuter then
        Dictionary False (List.map (\(k,v) -> (k, initHelp False v)) keyValuePairs)
      else if List.length keyValuePairs <= 8 then
        Dictionary False keyValuePairs
      else
        expando

    Record isClosed entries ->
      if isOuter then
        Record False (Dict.map (\_ v -> initHelp False v) entries)
      else if Dict.size entries <= 4 then
        Record False entries
      else
        expando

    Constructor maybeName isClosed args ->
      if isOuter then
        Constructor maybeName False (List.map (initHelp False) args)
      else if List.length args <= 4 then
        Constructor maybeName False args
      else
        expando



-- PRESERVE OLD EXPANDO STATE (open/closed)


merge : a -> Expando -> Expando
merge value expando =
  mergeHelp expando (Elm.Kernel.Debugger.init value)


mergeHelp : Expando -> Expando -> Expando
mergeHelp old new =
  case ( old, new ) of
    ( _, S _ ) ->
      new

    ( _, Primitive _ ) ->
      new

    ( Sequence _ isClosed oldValues, Sequence seqType _ newValues ) ->
      Sequence seqType isClosed (mergeListHelp oldValues newValues)

    ( Dictionary isClosed _, Dictionary _ keyValuePairs ) ->
      Dictionary isClosed keyValuePairs

    ( Record isClosed oldDict, Record _ newDict ) ->
      Record isClosed <| Dict.map (mergeDictHelp oldDict) newDict

    ( Constructor _ isClosed oldValues, Constructor maybeName _ newValues ) ->
      Constructor maybeName isClosed (mergeListHelp oldValues newValues)

    _ ->
      new


mergeListHelp : List Expando -> List Expando -> List Expando
mergeListHelp olds news =
  case (olds, news) of
    ( [], _ ) ->
      news

    ( _, [] ) ->
      news

    ( x :: xs, y :: ys ) ->
      mergeHelp x y :: mergeListHelp xs ys


mergeDictHelp : Dict String Expando -> String -> Expando -> Expando
mergeDictHelp oldDict key value =
  case Dict.get key oldDict of
    Nothing ->
      value

    Just oldValue ->
      mergeHelp oldValue value



-- UPDATE


type Msg
  = Toggle
  | Index Redirect Int Msg
  | Field String Msg


type Redirect = None | Key | Value


update : Msg -> Expando -> Expando
update msg value =
  case value of
    S _ ->
      value -- Debug.crash "nothing changes a primitive"

    Primitive _ ->
      value -- Debug.crash "nothing changes a primitive"

    Sequence seqType isClosed valueList ->
      case msg of
        Toggle ->
          Sequence seqType (not isClosed) valueList

        Index None index subMsg ->
          Sequence seqType isClosed <|
            updateIndex index (update subMsg) valueList

        Index _ _ _ ->
          value -- Debug.crash "no redirected indexes on sequences"

        Field _ _ ->
          value -- Debug.crash "no field on sequences"

    Dictionary isClosed keyValuePairs ->
      case msg of
        Toggle ->
          Dictionary (not isClosed) keyValuePairs

        Index redirect index subMsg ->
          case redirect of
            None ->
              value -- Debug.crash "must have redirect for dictionaries"

            Key ->
              Dictionary isClosed <|
                updateIndex index (\(k,v) -> (update subMsg k, v)) keyValuePairs

            Value ->
              Dictionary isClosed <|
                updateIndex index (\(k,v) -> (k, update subMsg v)) keyValuePairs

        Field _ _ ->
          value -- Debug.crash "no field for dictionaries"

    Record isClosed valueDict ->
      case msg of
        Toggle ->
          Record (not isClosed) valueDict

        Index _ _ _ ->
          value -- Debug.crash "no index for records"

        Field field subMsg ->
          Record isClosed (Dict.update field (updateField subMsg) valueDict)

    Constructor maybeName isClosed valueList ->
      case msg of
        Toggle ->
          Constructor maybeName (not isClosed) valueList

        Index None index subMsg ->
          Constructor maybeName isClosed <|
            updateIndex index (update subMsg) valueList

        Index _ _ _ ->
          value -- Debug.crash "no redirected indexes on sequences"

        Field _ _ ->
          value -- Debug.crash "no field for constructors"


updateIndex : Int -> (a -> a) -> List a -> List a
updateIndex n func list =
  case list of
    [] ->
      []

    x :: xs ->
      if n <= 0 then
        func x :: xs
      else
        x :: updateIndex (n-1) func xs


updateField : Msg -> Maybe Expando -> Maybe Expando
updateField msg maybeExpando =
  case maybeExpando of
    Nothing ->
      maybeExpando -- Debug.crash "key does not exist"

    Just expando ->
      Just (update msg expando)



-- VIEW


view : Maybe String -> Expando -> Html Msg
view maybeKey expando =
  case expando of
    S stringRep ->
      div (leftPad maybeKey) (lineStarter maybeKey Nothing [span [red] [text stringRep]])

    Primitive stringRep ->
      div (leftPad maybeKey) (lineStarter maybeKey Nothing [span [blue] [text stringRep]])

    Sequence seqType isClosed valueList ->
      viewSequence maybeKey seqType isClosed valueList

    Dictionary isClosed keyValuePairs ->
      viewDictionary maybeKey isClosed keyValuePairs

    Record isClosed valueDict ->
      viewRecord maybeKey isClosed valueDict

    Constructor maybeName isClosed valueList ->
      viewConstructor maybeKey maybeName isClosed valueList



-- VIEW SEQUENCE


viewSequence : Maybe String -> SeqType -> Bool -> List Expando -> Html Msg
viewSequence maybeKey seqType isClosed valueList =
  let
    starter =
      seqTypeToString (List.length valueList) seqType
  in
    div (leftPad maybeKey)
      [ div [ onClick Toggle ] (lineStarter maybeKey (Just isClosed) [text starter])
      , if isClosed then text "" else viewSequenceOpen valueList
      ]


viewSequenceOpen : List Expando -> Html Msg
viewSequenceOpen values =
  div [] (List.indexedMap viewConstructorEntry values)



-- VIEW DICTIONARY


viewDictionary : Maybe String -> Bool -> List (Expando, Expando) -> Html Msg
viewDictionary maybeKey isClosed keyValuePairs =
  let
    starter =
      "Dict(" ++ String.fromInt (List.length keyValuePairs) ++ ")"
  in
    div (leftPad maybeKey)
      [ div [ onClick Toggle ] (lineStarter maybeKey (Just isClosed) [text starter])
      , if isClosed then text "" else viewDictionaryOpen keyValuePairs
      ]


viewDictionaryOpen : List (Expando, Expando) -> Html Msg
viewDictionaryOpen keyValuePairs =
  div [] (List.indexedMap viewDictionaryEntry keyValuePairs)


viewDictionaryEntry : Int -> (Expando, Expando) -> Html Msg
viewDictionaryEntry index (key, value) =
  case key of
    S stringRep ->
      Html.map (Index Value index) (view (Just stringRep) value)

    Primitive stringRep ->
      Html.map (Index Value index) (view (Just stringRep) value)

    _ ->
        div []
          [ Html.map (Index Key index) (view (Just "key") key)
          , Html.map (Index Value index) (view (Just "value") value)
          ]



-- VIEW RECORD


viewRecord : Maybe String -> Bool -> Dict String Expando -> Html Msg
viewRecord maybeKey isClosed record =
  let
    (start, middle, end) =
      if isClosed then
        ( Tuple.second (viewTinyRecord record), text "", text "" )
      else
        ( [ text "{" ], viewRecordOpen record, div (leftPad (Just ())) [text "}"] )
  in
    div (leftPad maybeKey)
      [ div [ onClick Toggle ] (lineStarter maybeKey (Just isClosed) start)
      , middle
      , end
      ]


viewRecordOpen : Dict String Expando -> Html Msg
viewRecordOpen record =
  div [] (List.map viewRecordEntry (Dict.toList record))


viewRecordEntry : (String, Expando) -> Html Msg
viewRecordEntry (field, value) =
  Html.map (Field field) (view (Just field) value)



-- VIEW CONSTRUCTOR


viewConstructor : Maybe String -> Maybe String -> Bool -> List Expando -> Html Msg
viewConstructor maybeKey maybeName isClosed valueList =
  let
    tinyArgs =
      List.map (Tuple.second << viewExtraTiny) valueList

    description =
      case (maybeName, tinyArgs) of
        (Nothing, []) ->
          [ text "()" ]

        (Nothing, x :: xs) ->
          text "( "
            :: span [] x
            :: List.foldr (\args rest -> text ", " :: span [] args :: rest) [text " )"] xs

        (Just name, []) ->
          [ text name ]

        (Just name, x :: xs) ->
          text (name ++ " ")
            :: span [] x
            :: List.foldr (\args rest -> text " " :: span [] args :: rest) [] xs

    (maybeIsClosed, openHtml) =
      case valueList of
        [] ->
          ( Nothing, div [] [] )

        [entry] ->
          case entry of
            S _ ->
              ( Nothing, div [] [] )

            Primitive _ ->
              ( Nothing, div [] [] )

            Sequence _ _ subValueList ->
              ( Just isClosed
              , if isClosed then div [] [] else Html.map (Index None 0) (viewSequenceOpen subValueList)
              )

            Dictionary _ keyValuePairs ->
              ( Just isClosed
              , if isClosed then div [] [] else Html.map (Index None 0) (viewDictionaryOpen keyValuePairs)
              )

            Record _ record ->
              ( Just isClosed
              , if isClosed then div [] [] else Html.map (Index None 0) (viewRecordOpen record)
              )

            Constructor _ _ subValueList ->
              ( Just isClosed
              , if isClosed then div [] [] else Html.map (Index None 0) (viewConstructorOpen subValueList)
              )

        _ ->
          ( Just isClosed
          , if isClosed then div [] [] else viewConstructorOpen valueList
          )
  in
    div (leftPad maybeKey)
      [ div [ onClick Toggle ] (lineStarter maybeKey maybeIsClosed description)
      , openHtml
      ]


viewConstructorOpen : List Expando -> Html Msg
viewConstructorOpen valueList =
  div [] (List.indexedMap viewConstructorEntry valueList)


viewConstructorEntry : Int -> Expando -> Html Msg
viewConstructorEntry index value =
  Html.map (Index None index) (view (Just (String.fromInt index)) value)



-- VIEW TINY


viewTiny : Expando -> ( Int, List (Html msg) )
viewTiny value =
  case value of
    S stringRep ->
      let
        str =
          elideMiddle stringRep
      in
        ( String.length str
        , [ span [red] [text str] ]
        )

    Primitive stringRep ->
      ( String.length stringRep
      , [ span [blue] [text stringRep] ]
      )

    Sequence seqType _ valueList ->
      viewTinyHelp <|
        seqTypeToString (List.length valueList) seqType

    Dictionary _ keyValuePairs ->
      viewTinyHelp <|
        "Dict(" ++ String.fromInt (List.length keyValuePairs) ++ ")"

    Record _ record ->
      viewTinyRecord record

    Constructor maybeName _ [] ->
      viewTinyHelp <|
        Maybe.withDefault "Unit" maybeName

    Constructor maybeName _ valueList ->
      viewTinyHelp <|
        case maybeName of
          Nothing ->
            "Tuple(" ++ String.fromInt (List.length valueList) ++ ")"

          Just name ->
            name ++ " …"


viewTinyHelp : String -> ( Int, List (Html msg) )
viewTinyHelp str =
  ( String.length str, [text str] )


elideMiddle : String -> String
elideMiddle str =
  if String.length str <= 18 then
    str

  else
    String.left 8 str ++ "..." ++ String.right 8 str



-- VIEW TINY RECORDS


viewTinyRecord : Dict String Expando -> ( Int, List (Html msg) )
viewTinyRecord record =
  if Dict.isEmpty record then
    ( 2, [text "{}"] )

  else
    viewTinyRecordHelp 0 "{ " (Dict.toList record)


viewTinyRecordHelp : Int -> String -> List (String, Expando) -> ( Int, List (Html msg) )
viewTinyRecordHelp length starter entries =
  case entries of
    [] ->
      ( length + 2, [ text " }" ] )

    (field, value) :: rest ->
      let
        fieldLen =
          String.length field

        (valueLen, valueHtmls) =
          viewExtraTiny value

        newLength =
          length + fieldLen + valueLen + 5
      in
        if newLength > 60 then
          ( length + 4, [text ", … }"] )

        else
          let
            ( finalLength, otherHtmls ) =
              viewTinyRecordHelp newLength ", " rest
          in
            ( finalLength
            , text starter
              :: span [purple] [text field]
              :: text " = "
              :: span [] valueHtmls
              :: otherHtmls
            )


viewExtraTiny : Expando -> ( Int, List (Html msg) )
viewExtraTiny value =
  case value of
    Record _ record ->
      viewExtraTinyRecord 0 "{" (Dict.keys record)

    _ ->
      viewTiny value


viewExtraTinyRecord : Int -> String -> List String -> ( Int, List (Html msg) )
viewExtraTinyRecord length starter entries =
  case entries of
    [] ->
      ( length + 1, [text "}"] )

    field :: rest ->
      let
        nextLength =
          length + String.length field + 1
      in
        if nextLength > 18 then
          ( length + 2, [text "…}"])

        else
          let
            (finalLength, otherHtmls) =
              viewExtraTinyRecord nextLength "," rest
          in
            ( finalLength
            , text starter :: span [purple] [text field] :: otherHtmls
            )



-- VIEW HELPERS


lineStarter : Maybe String -> Maybe Bool -> List (Html msg) -> List (Html msg)
lineStarter maybeKey maybeIsClosed description =
  let
    arrow =
      case maybeIsClosed of
        Nothing ->
          makeArrow ""

        Just True ->
          makeArrow "▸"

        Just False ->
          makeArrow "▾"
  in
    case maybeKey of
      Nothing ->
        arrow :: description

      Just key ->
        arrow :: span [purple] [text key] :: text " = " :: description


makeArrow : String -> Html msg
makeArrow arrow =
  span
    [ style "color" "#777"
    , style "padding-left" "2ch"
    , style "width" "2ch"
    , style "display" "inline-block"
    ]
    [ text arrow ]


leftPad : Maybe a -> List (Html.Attribute msg)
leftPad maybeKey =
  case maybeKey of
    Nothing ->
      []

    Just _ ->
      [ style "padding-left" "4ch" ]


red : Html.Attribute msg
red =
  style "color" "rgb(196, 26, 22)"


blue : Html.Attribute msg
blue =
  style "color" "rgb(28, 0, 207)"


purple : Html.Attribute msg
purple =
  style "color" "rgb(136, 19, 145)"
