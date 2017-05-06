module Views.AutoComplete.Updates exposing (update)

import Views.AutoComplete.Types exposing (Model, Msg(..))
import Views.AutoComplete.Match exposing (levenshteinFromStrings)
import Task
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

        PressingBackspace pressed ->
            ( model, Cmd.none )

        UpdateFieldText text ->
            updateAutoComplete { model | fieldText = text }

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
                Set.toList model.list
                    |> List.sortBy (levenshteinFromStrings model.fieldText)
                    |> List.take 10
      }
    , Cmd.batch
        [ Cmd.none ]
    )
