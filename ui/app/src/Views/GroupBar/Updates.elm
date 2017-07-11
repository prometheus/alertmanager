module Views.GroupBar.Updates exposing (update, setFields)

import Views.GroupBar.Types exposing (Model, Msg(..))
import Utils.Match exposing (jaroWinkler)
import Task
import Dom
import Set
import Utils.Filter exposing (Filter, generateQueryString, stringifyGroup, parseGroup)
import Navigation


update : String -> Filter -> Msg -> Model -> ( Model, Cmd Msg )
update url filter msg model =
    case msg of
        AddField emptyFieldText text ->
            immediatelyFilter url
                filter
                { model
                    | fields = model.fields ++ [ text ]
                    , matches = []
                    , fieldText =
                        if emptyFieldText then
                            ""
                        else
                            model.fieldText
                }

        DeleteField setFieldText text ->
            immediatelyFilter url
                filter
                { model
                    | fields = List.filter ((/=) text) model.fields
                    , matches = []
                    , fieldText =
                        if setFieldText then
                            text
                        else
                            model.fieldText
                }

        Select maybeSelectedMatch ->
            ( { model | maybeSelectedMatch = maybeSelectedMatch }, Cmd.none )

        Focus focused ->
            ( { model
                | focused = focused
                , maybeSelectedMatch = Nothing
              }
            , Cmd.none
            )

        ResultsHovered resultsHovered ->
            ( { model
                | resultsHovered = resultsHovered
              }
            , Cmd.none
            )

        PressingBackspace pressed ->
            ( { model | backspacePressed = pressed }, Cmd.none )

        UpdateFieldText text ->
            updateAutoComplete
                { model
                    | fieldText = text
                }

        Noop ->
            ( model, Cmd.none )


immediatelyFilter : String -> Filter -> Model -> ( Model, Cmd Msg )
immediatelyFilter url filter model =
    let
        newFilter =
            { filter | group = stringifyGroup model.fields }
    in
        ( model
        , Cmd.batch
            [ Navigation.newUrl (url ++ generateQueryString newFilter)
            , Dom.focus "group-by-field" |> Task.attempt (always Noop)
            ]
        )


setFields : Filter -> Model -> Model
setFields filter model =
    { model
        | fields =
            parseGroup filter.group
    }


updateAutoComplete : Model -> ( Model, Cmd Msg )
updateAutoComplete model =
    ( { model
        | matches =
            if String.isEmpty model.fieldText then
                []
            else if String.contains " " model.fieldText then
                model.matches
            else
                -- TODO: How many matches do we want to show?
                -- NOTE: List.reverse is used because our scale is (0.0, 1.0),
                -- but we want the higher values to be in the front of the
                -- list.
                Set.toList model.list
                    |> List.filter ((flip List.member model.fields) >> not)
                    |> List.sortBy (jaroWinkler model.fieldText)
                    |> List.reverse
                    |> List.take 10
        , maybeSelectedMatch = Nothing
      }
    , Cmd.none
    )
