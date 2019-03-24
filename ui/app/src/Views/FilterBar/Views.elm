module Views.FilterBar.Views exposing (view)

import Html exposing (Attribute, Html, button, div, i, input, small, span, text)
import Html.Attributes exposing (class, disabled, id, style, value)
import Html.Events exposing (keyCode, on, onClick, onInput)
import Utils.Filter exposing (Matcher)
import Utils.Keyboard exposing (keys, onKeyDown, onKeyUp)
import Utils.List
import Views.FilterBar.Types exposing (Model, Msg(..))


keys :
    { backspace : Int
    , enter : Int
    }
keys =
    { backspace = 8
    , enter = 13
    }


viewMatcher : Matcher -> Html Msg
viewMatcher matcher =
    div [ class "col col-auto" ]
        [ div [ class "btn-group mr-2 mb-2" ]
            [ button
                [ class "btn btn-outline-info"
                , onClick (DeleteFilterMatcher True matcher)
                ]
                [ text <| Utils.Filter.stringifyMatcher matcher
                ]
            , button
                [ class "btn btn-outline-danger"
                , onClick (DeleteFilterMatcher False matcher)
                ]
                [ text "×" ]
            ]
        ]


viewMatchers : List Matcher -> List (Html Msg)
viewMatchers matchers =
    matchers
        |> List.map viewMatcher


view : Model -> Html Msg
view { matchers, matcherText, backspacePressed } =
    let
        maybeMatcher =
            Utils.Filter.parseMatcher matcherText

        maybeLastMatcher =
            Utils.List.lastElem matchers

        className =
            if matcherText == "" then
                ""

            else
                case maybeMatcher of
                    Just _ ->
                        "has-success"

                    Nothing ->
                        "has-danger"

        keyDown key =
            if key == keys.enter then
                maybeMatcher
                    |> Maybe.map (AddFilterMatcher True)
                    |> Maybe.withDefault Noop

            else if key == keys.backspace then
                if matcherText == "" then
                    case ( backspacePressed, maybeLastMatcher ) of
                        ( False, Just lastMatcher ) ->
                            DeleteFilterMatcher True lastMatcher

                        _ ->
                            Noop

                else
                    PressingBackspace True

            else
                Noop

        keyUp key =
            if key == keys.backspace then
                PressingBackspace False

            else
                Noop

        onClickAttr =
            maybeMatcher
                |> Maybe.map (AddFilterMatcher True)
                |> Maybe.withDefault Noop
                |> onClick

        isDisabled =
            maybeMatcher == Nothing
    in
    div
        [ class "row no-gutters align-items-start" ]
        (viewMatchers matchers
            ++ [ div
                    [ class ("col " ++ className)
                    , style "min-width" "200px"
                    ]
                    [ div [ class "input-group" ]
                        [ input
                            [ id "filter-bar-matcher"
                            , class "form-control"
                            , value matcherText
                            , onKeyDown keyDown
                            , onKeyUp keyUp
                            , onInput UpdateMatcherText
                            ]
                            []
                        , span
                            [ class "input-group-btn" ]
                            [ button [ class "btn btn-primary", disabled isDisabled, onClickAttr ] [ text "+" ] ]
                        ]
                    , small [ class "form-text text-muted" ]
                        [ text "Custom matcher, e.g."
                        , button
                            [ class "btn btn-link btn-sm align-baseline"
                            , onClick (UpdateMatcherText exampleMatcher)
                            ]
                            [ text exampleMatcher ]
                        ]
                    ]
               ]
        )


exampleMatcher : String
exampleMatcher =
    "env=\"production\""
