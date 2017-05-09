module Views.AutoComplete.Updates exposing (update)

import Views.AutoComplete.Types exposing (Model, Msg(..))
import Views.AutoComplete.Match exposing (jaroWinkler)
import Task
import Time
import Process
import Dom
import Set


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    case msg of
        AddField emptyFieldText text ->
            ( { model
                | fields = model.fields ++ [ text ]
                , matches = []
                , fieldText =
                    if emptyFieldText then
                        ""
                    else
                        model.fieldText
              }
            , Dom.focus "auto-complete-field" |> Task.attempt (always Noop)
            )

        DeleteField setFieldText text ->
            ( { model
                | fields = List.filter ((/=) text) model.fields
                , matches = []
                , fieldText =
                    if setFieldText then
                        text
                    else
                        model.fieldText
              }
            , Dom.focus "auto-complete-field" |> Task.attempt (always Noop)
            )

        Select maybeSelectedMatch ->
            ( { model | maybeSelectedMatch = maybeSelectedMatch }, Cmd.none )

        Focus focused ->
            ( { model
                | focused = focused
                , maybeSelectedMatch = Nothing
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


updateAutoComplete : Model -> ( Model, Cmd Msg )
updateAutoComplete model =
    ( { model
        | matches =
            if String.isEmpty model.fieldText then
                []
            else if String.contains " " model.fieldText then
                model.matches
            else
                -- TODO: Disallow adding spaces, or only check distance if
                -- there are no spaces.
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
    , Cmd.batch
        [ Cmd.none ]
    )
